import { Box, Button, Typography, Paper } from '@mui/material';
import GitHubIcon from '@mui/icons-material/GitHub';

interface LoginProps {
  // The onLogin prop is not being used, so we're removing it
}

const Login = ({}: LoginProps) => {
  const handleGitHubLogin = () => {
    // Redirect to GitHub auth endpoint
    window.location.href = '/auth/login';
  };

  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        minHeight: '60vh',
      }}
    >
      <Paper
        elevation={3}
        sx={{
          p: 4,
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          maxWidth: 400,
          width: '100%',
        }}
      >
        <Typography variant="h4" component="h1" gutterBottom>
          Welcome to Teamspace Manager
        </Typography>
        <Typography variant="body1" color="text.secondary" align="center" sx={{ mb: 3 }}>
          Please sign in with your GitHub account to access your teamspaces
        </Typography>
        <Button
          variant="contained"
          startIcon={<GitHubIcon />}
          onClick={handleGitHubLogin}
          size="large"
          sx={{ width: '100%' }}
        >
          Sign in with GitHub
        </Button>
      </Paper>
    </Box>
  );
};

export default Login; 