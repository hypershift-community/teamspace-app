import { useEffect, useState, useMemo, useCallback } from 'react';
import api from './api';
import './App.css';
import { Dialog, DialogActions, DialogContent, DialogTitle, TextField, MenuItem } from '@mui/material';
import { Button } from '@mui/material';

interface Teamspace {
  name: string;
  namespace: string;
  createdAt: string;
  deletionTimestamp?: string;
  isDeleting?: boolean;
}

function App() {
  const [isAuthenticated, setIsAuthenticated] = useState<boolean | null>(null);
  const [username, setUsername] = useState<string | null>(null);
  const [teamspaces, setTeamspaces] = useState<Teamspace[]>([]);
  const [fetchingError, setFetchingError] = useState<string | null>(null);
  const [copiedCommand, setCopiedCommand] = useState<string | null>(null);
  const [open, setOpen] = useState(false);
  const [newTeamspaceName, setNewTeamspaceName] = useState('');
  const [newInitialHostedClusterRelease, setNewInitialHostedClusterRelease] = useState('quay.io/openshift-release-dev/ocp-release:4.19.0-ec.5-multi');
  const [featureSet, setFeatureSet] = useState('Default');

  // Check if any teamspace is in deleting state for polling
  const hasDeleteInProgress = useMemo(() => {
    return teamspaces.some(ts => ts.isDeleting || !!ts.deletionTimestamp);
  }, [teamspaces]);

  // Function to fetch teamspaces
  const fetchTeamspaces = useCallback(async () => {
    try {
      setFetchingError(null);
      const response = await api.get('/api/teamspaces');
      setTeamspaces(prev => {
        return response.data?.map((ts: Teamspace) => {
          // Preserve the isDeleting flag from previous state
          const prevTeamspace = prev.find(p => p.name === ts.name);
          const isBeingDeleted = !!ts.deletionTimestamp || (prevTeamspace?.isDeleting === true);
          return {
            ...ts,
            isDeleting: isBeingDeleted
          };
        }) || [];
      });
    } catch (err) {
      console.error('Failed to fetch teamspaces:', err);
      setFetchingError('Failed to load teamspaces');
    }
  }, []);

  // Polling effect
  useEffect(() => {
    // If any deletion is in progress, poll more frequently
    if (hasDeleteInProgress) {
      const interval = setInterval(() => {
        console.log('Polling for teamspace status during deletion...');
        fetchTeamspaces();
      }, 3000); // Poll every 3 seconds when deletion is in progress
      
      return () => clearInterval(interval);
    }
  }, [hasDeleteInProgress, fetchTeamspaces]);

  // Initial auth status check
  useEffect(() => {
    const checkAuth = async () => {
      try {
        const response = await api.get('/auth/status', {
          withCredentials: true // This ensures cookies are sent
        });
        
        console.log('Authentication status:', response.data);
        
        if (response.data.authenticated) {
          setIsAuthenticated(true);
          if (response.data.username) {
            setUsername(response.data.username);
          }
          
          // Load teamspaces if authenticated
          fetchTeamspaces();
        } else {
          setIsAuthenticated(false);
          setUsername('');
          setTeamspaces([]);
        }
      } catch (err) {
        console.error('Authentication check failed:', err);
        setFetchingError('Failed to check authentication status');
        setIsAuthenticated(false);
        setUsername('');
        setTeamspaces([]);
      }
    };
    
    checkAuth();
  }, [fetchTeamspaces]);

  const handleOpen = () => setOpen(true);
  const handleClose = () => setOpen(false);

  const handleCreate = async () => {
    const featureSetValue = featureSet === 'Default' ? '' : featureSet;

    if (!newTeamspaceName || !newInitialHostedClusterRelease) {
      alert('Please fill in all fields');
      return;
    }

    try {
      console.log('Creating teamspace:', newTeamspaceName);
      const createResponse = await api.post('/api/teamspaces', {
        name: newTeamspaceName,
        initialHostedClusterRelease: newInitialHostedClusterRelease,
        featureSet: featureSetValue
      });
      console.log('Create response:', createResponse.data);
      const response = await api.get('/api/teamspaces');
      setTeamspaces(response.data || []);
      alert(`Teamspace "${newTeamspaceName}" created successfully!`);
      handleClose();
    } catch (err) {
      console.error('Failed to create teamspace:', err);
      alert('Failed to create teamspace: ' + (err instanceof Error ? err.message : 'Unknown error'));
    }
  };

  const handleDeleteTeamspace = async (name: string) => {
    if (!isAuthenticated) {
      alert('Please login first');
      return;
    }

    if (!confirm(`Are you sure you want to delete teamspace "${name}"?`)) {
      console.log('Teamspace deletion cancelled');
      return;
    }

    try {
      console.log('Deleting teamspace:', name);
      
      // Mark the teamspace as deleting in the UI immediately
      setTeamspaces(prev => 
        prev.map(ts => 
          ts.name === name ? { ...ts, isDeleting: true } : ts
        )
      );
      
      // Send delete request to backend
      await api.delete(`/api/teamspaces/${name}`);
      
      // Fetch updated teamspaces that include deletion timestamps
      await fetchTeamspaces();
      
      console.log(`Teamspace "${name}" deletion initiated!`);
    } catch (err) {
      console.error('Failed to delete teamspace:', err);
      
      // If delete fails, remove the deleting flag
      setTeamspaces(prev => 
        prev.map(ts => 
          ts.name === name ? { ...ts, isDeleting: false } : ts
        )
      );
      
      alert('Failed to delete teamspace: ' + (err instanceof Error ? err.message : 'Unknown error'));
    }
  };

  const handleLogout = async () => {
    try {
      // Show loading state
      setIsAuthenticated(false);
      setUsername('');
      setTeamspaces([]);
      
      // Clear any localStorage/sessionStorage data
      localStorage.removeItem('auth_state');
      sessionStorage.clear();
      
      // Clear cookies by setting an early expiration date
      document.cookie.split(";").forEach(function(c) {
        document.cookie = c.replace(/^ +/, "")
          .replace(/=.*/, "=;expires=" + new Date().toUTCString() + ";path=/");
      });
      
      // Navigate to login page instead of reload to prevent potential auto-login issues
      window.location.href = '/';
    } catch (err) {
      console.error('Logout failed:', err);
      
      // Even if the server request fails, still logout on the client side
      setIsAuthenticated(false);
      setUsername('');
      setTeamspaces([]);
      
      // Show a brief error message
      setFetchingError('Server logout failed, but you have been logged out of this session');
      setTimeout(() => setFetchingError(null), 3000);
      
      // Force navigation to home
      window.location.href = '/';
    }
  };

  const copyToClipboard = (command: string, teamspaceName: string) => {
    navigator.clipboard.writeText(command).then(
      () => {
        setCopiedCommand(teamspaceName);
        setTimeout(() => setCopiedCommand(null), 2000); // Reset after 2 seconds
      },
      (err) => {
        console.error('Could not copy text: ', err);
      }
    );
  };

  const downloadKubeconfig = async (teamspaceName: string) => {
    try {
      const response = await fetch(`/api/teamspaces/${teamspaceName}/kubeconfig`, {
        method: 'GET',
        credentials: 'include', // Include cookies for authentication
      });

      if (!response.ok) {
        throw new Error('Network response was not ok');
      }

      const blob = await response.blob();
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `kubeconfig-${teamspaceName}.yaml`;
      document.body.appendChild(a);
      a.click();
      a.remove();
      window.URL.revokeObjectURL(url);
    } catch (error) {
      console.error('Failed to download kubeconfig:', error);
      alert('Failed to download kubeconfig');
    }
  };

  if (isAuthenticated === null) {
    return <div className="loading">Loading...</div>;
  }

  return (
    <div className="app">
      <header>
        <div className="auth-container">
          {fetchingError ? (
            <div className="error">{fetchingError}</div>
          ) : (
            <>
              {isAuthenticated ? (
                <>
                  <div className="welcome-message">
                    {`Welcome, ${username || 'User'}!`}
                  </div>
                  <button onClick={handleLogout} className="btn btn-danger">
                    Logout
                  </button>
                </>
              ) : (
                <a href="/auth/login" className="btn">
                  Login with GitHub
                </a>
              )}
            </>
          )}
        </div>

        <h1>Teamspace Manager</h1>
        <p className="header-description">
          Each Teamspace will give you a kubeconfig to a management cluster with a brand new HostedCluster. 
          <br />
          Use the providerd "Commands" to quickly test your component on it. 
          <br />
          <br />
          If you are a more advance use case, checkout <a href="https://hypershift.pages.dev/getting-started/" target="_blank" rel="noopener noreferrer" className="header-link">HyperShift documentation</a> for creating more HCs using your own infra or installing the HO on the dev HC to use it as your own management cluster.
        </p>
      </header>
      
      <main>
        {isAuthenticated ? (
          <div className="teamspaces-container">
            <h2>Your Teamspaces</h2>
            <button onClick={handleOpen} className="btn">
              Create New Teamspace
            </button>

            <Dialog open={open} onClose={handleClose}>
              <DialogTitle>Create New Teamspace</DialogTitle>
              <DialogContent>
                <TextField
                  autoFocus
                  margin="dense"
                  label="Teamspace Name"
                  type="text"
                  fullWidth
                  value={newTeamspaceName}
                  onChange={(e) => setNewTeamspaceName(e.target.value)}
                />
                <TextField
                  margin="dense"
                  label="Initial HostedCluster Release"
                  type="text"
                  fullWidth
                  value={newInitialHostedClusterRelease}
                  onChange={(e) => setNewInitialHostedClusterRelease(e.target.value)}
                />
                <TextField
                  select
                  label="FeatureSet"
                  value={featureSet || 'Default'}
                  onChange={(e) => setFeatureSet(e.target.value)}
                  fullWidth
                  margin="dense"
                >
                  <MenuItem value="Default">Default</MenuItem>
                  <MenuItem value="TechPreviewNoUpgrade">TechPreviewNoUpgrade</MenuItem>
                  <MenuItem value="DevPreviewNoUpgrade">DevPreviewNoUpgrade</MenuItem>
                </TextField>
              </DialogContent>
              <DialogActions>
                <Button onClick={handleClose} color="primary">
                  Cancel
                </Button>
                <Button onClick={handleCreate} color="primary">
                  Create
                </Button>
              </DialogActions>
            </Dialog>

            <div className="teamspaces">
              {teamspaces.length === 0 ? (
                <p>No teamspaces found. Create one to get started.</p>
              ) : (
                teamspaces.map((teamspace) => (
                  <div key={teamspace.namespace} className="teamspace-card">
                    <h3>
                      {teamspace.name}
                      {(teamspace.isDeleting || teamspace.deletionTimestamp) && (
                        <span className="deleting-indicator">
                          <span className="spinner"></span>
                          Deleting...
                        </span>
                      )}
                    </h3>
                    <p>Namespace: {teamspace.namespace}</p>
                    <p>Created: {new Date(teamspace.createdAt).toLocaleString()}</p>
                    
                    <div className="commands-section">
                      <h4>Commands</h4>
                      <div className="command-item">
                        <p>Export the teamspace kubeconfig:</p>
                        <div className="command-box">
                          <code>export KUBECONFIG=~/Downloads/kubeconfig-{teamspace.name}.yaml</code>
                          <button 
                            className="copy-btn"
                            onClick={() => copyToClipboard(`export KUBECONFIG=~/Downloads/kubeconfig-${teamspace.name}.yaml`, `export-${teamspace.name}`)}
                          >
                            {copiedCommand === `export-${teamspace.name}` ? 'Copied!' : 'Copy'}
                          </button>
                        </div>
                      </div>
                      
                      <div className="command-item">
                        <p>Use your component custom image:</p>
                        <div className="command-box">
                          <code>kubectl annotate hc dev hypershift.openshift.io/image-overrides="cluster-ingress-operator=example.com/cno:latest"</code>
                          <button 
                            className="copy-btn"
                            onClick={() => copyToClipboard(`kubectl annotate hc dev hypershift.openshift.io/image-overrides="cluster-ingress-operator=example.com/cno:latest"`, teamspace.name)}
                          >
                            {copiedCommand === teamspace.name ? 'Copied!' : 'Copy'}
                          </button>
                        </div>
                      </div>
                        
                      <div className="command-item">
                        <p>Restore component images:</p>
                        <div className="command-box">
                          <code>kubectl annotate hc dev hypershift.openshift.io/image-overrides="" --overwrite</code>
                          <button 
                            className="copy-btn"
                            onClick={() => copyToClipboard(`kubectl annotate hc dev hypershift.openshift.io/image-overrides="" --overwrite`, `restore-${teamspace.name}`)}
                          >
                            {copiedCommand === `restore-${teamspace.name}` ? 'Copied!' : 'Copy'}
                          </button>
                        </div>
                      </div>
                     
                      <div className="command-item">
                        <p>Inspect control plane pods:</p>
                        <div className="command-box">
                          <code>kubectl get pods -nteamspace-{teamspace.name}-dev</code>
                          <button 
                            className="copy-btn"
                            onClick={() => copyToClipboard(`kubectl get pods -nteamspace-${teamspace.name}-dev`, `pods-${teamspace.name}`)}
                          >
                            {copiedCommand === `pods-${teamspace.name}` ? 'Copied!' : 'Copy'}
                          </button>
                        </div>
                      </div>

                      <div className="command-item">
                        <p>Get the HostedCluster kubeconfig:</p>
                        <div className="command-box">
                          <code>kubectl get secret -n {teamspace.namespace} dev-admin-kubeconfig -o jsonpath='{'{.data.kubeconfig}'}' | base64 -d &gt; kubeconfig-{teamspace.name}-hostedcluster</code>
                          <button 
                            className="copy-btn"
                            onClick={() => copyToClipboard(`kubectl get secret -n ${teamspace.namespace} dev-admin-kubeconfig -o jsonpath='{.data.kubeconfig}' | base64 -d > kubeconfig-${teamspace.name}-hostedcluster`, `hostedcluster-${teamspace.name}`)}
                          >
                            {copiedCommand === `hostedcluster-${teamspace.name}` ? 'Copied!' : 'Copy'}
                          </button>
                        </div>
                      </div>
                    </div>
                      
                    <div className="teamspace-actions">
                      <button
                        onClick={() => downloadKubeconfig(teamspace.name)}
                        className="btn"
                        style={{ 
                          opacity: (teamspace.isDeleting || teamspace.deletionTimestamp) ? 0.5 : 1,
                          pointerEvents: (teamspace.isDeleting || teamspace.deletionTimestamp) ? 'none' : 'auto'
                        }}
                        disabled={teamspace.isDeleting || !!teamspace.deletionTimestamp}
                      >
                        Download Kubeconfig
                      </button>
                      <button
                        onClick={() => handleDeleteTeamspace(teamspace.name)}
                        className="btn btn-danger"
                        disabled={teamspace.isDeleting || !!teamspace.deletionTimestamp}
                      >
                        {(teamspace.isDeleting || teamspace.deletionTimestamp) ? 'Deleting...' : 'Delete'}
                      </button>
                    </div>
                  </div>
                ))
              )}
            </div>
          </div>
        ) : (
          <div className="login-container">
            <p>Please login to view and manage your teamspaces</p>
          </div>
        )}
      </main>
    </div>
  );
}

export default App;
