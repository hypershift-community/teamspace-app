package kubernetes

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type Teamspace struct {
	Name              string     `json:"name"`
	Namespace         string     `json:"namespace"`
	CreatedAt         time.Time  `json:"createdAt"`
	Owner             string     `json:"owner"`
	DeletionTimestamp *time.Time `json:"deletionTimestamp,omitempty"`
}

type TeamspaceManager struct {
	clientset *kubernetes.Clientset
}

func NewTeamspaceManager() (*TeamspaceManager, error) {
	var config *rest.Config
	var err error

	// Try in-cluster config first
	config, err = rest.InClusterConfig()
	if err != nil {
		// If in-cluster config fails, try loading from kubeconfig
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		configOverrides := &clientcmd.ConfigOverrides{}
		kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
		config, err = kubeConfig.ClientConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to get Kubernetes config: %v", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %v", err)
	}

	return &TeamspaceManager{
		clientset: clientset,
	}, nil
}

func (m *TeamspaceManager) CreateTeamspace(name string, owner string, initialHostedClusterRelease string, featureSet string) (*Teamspace, error) {
	namespace := fmt.Sprintf("teamspace-%s", name)
	teamspace := &Teamspace{
		Name:      name,
		Namespace: namespace,
		CreatedAt: time.Now(),
		Owner:     owner,
	}

	// Create namespace
	_, err := m.clientset.CoreV1().Namespaces().Create(context.TODO(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
			Labels: map[string]string{
				"teamspace": "true",
				"owner":     owner,
				"name":      name,
			},
			Annotations: map[string]string{
				"release":     initialHostedClusterRelease,
				"feature-set": featureSet,
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create namespace: %v", err)
	}

	return teamspace, nil
}

func (m *TeamspaceManager) DeleteTeamspace(name string) error {
	namespace := fmt.Sprintf("teamspace-%s", name)
	return m.clientset.CoreV1().Namespaces().Delete(context.TODO(), namespace, metav1.DeleteOptions{})
}

func (m *TeamspaceManager) ListTeamspaces() ([]*Teamspace, error) {
	namespaces, err := m.clientset.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{
		LabelSelector: "teamspace=true",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %v", err)
	}

	var teamspaces []*Teamspace
	for _, ns := range namespaces.Items {
		teamspace := &Teamspace{
			Name:      ns.Labels["name"],
			Namespace: ns.Name,
			CreatedAt: ns.CreationTimestamp.Time,
			Owner:     ns.Labels["owner"],
		}

		// Include deletion timestamp if the namespace is being deleted
		if ns.DeletionTimestamp != nil {
			deletionTime := ns.DeletionTimestamp.Time
			teamspace.DeletionTimestamp = &deletionTime
		}

		teamspaces = append(teamspaces, teamspace)
	}

	return teamspaces, nil
}

// ListTeamspacesByOwner lists teamspaces owned by a specific user
func (m *TeamspaceManager) ListTeamspacesByOwner(owner string) ([]*Teamspace, error) {
	namespaces, err := m.clientset.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("teamspace=true,owner=%s", owner),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %v", err)
	}

	var teamspaces []*Teamspace
	for _, ns := range namespaces.Items {
		teamspace := &Teamspace{
			Name:      ns.Labels["name"],
			Namespace: ns.Name,
			CreatedAt: ns.CreationTimestamp.Time,
			Owner:     owner,
		}

		// Include deletion timestamp if the namespace is being deleted
		if ns.DeletionTimestamp != nil {
			deletionTime := ns.DeletionTimestamp.Time
			teamspace.DeletionTimestamp = &deletionTime
		}

		teamspaces = append(teamspaces, teamspace)
	}

	return teamspaces, nil
}

func (m *TeamspaceManager) GetKubeconfig(name string) ([]byte, error) {
	namespace := fmt.Sprintf("teamspace-%s", name)
	kubeconfigSecret := fmt.Sprintf("teamspace-%s-kubeconfig", name)
	secret, err := m.clientset.CoreV1().Secrets(namespace).Get(context.TODO(), kubeconfigSecret, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig secret: %v", err)
	}

	return secret.Data["kubeconfig"], nil
}

// IsTeamspaceOwner checks if the given user is the owner of the specified teamspace
func (m *TeamspaceManager) IsTeamspaceOwner(name string, username string) (bool, error) {
	namespace := fmt.Sprintf("teamspace-%s", name)
	ns, err := m.clientset.CoreV1().Namespaces().Get(context.TODO(), namespace, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to get namespace: %v", err)
	}

	owner, exists := ns.Labels["owner"]
	if !exists {
		return false, fmt.Errorf("namespace %s has no owner label", namespace)
	}

	return owner == username, nil
}
