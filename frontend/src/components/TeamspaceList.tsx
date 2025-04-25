import { useState, useEffect } from 'react';
import { Box, Typography, Button, Card, CardContent, CardActions } from '@mui/material';
import { Link } from 'react-router-dom';
import api from '../api';

interface Teamspace {
  id: string;
  name: string;
  description: string;
}

function TeamspaceList() {
  const [teamspaces, setTeamspaces] = useState<Teamspace[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const fetchTeamspaces = async () => {
      try {
        const response = await api.get('/teamspaces');
        setTeamspaces(response.data);
      } catch (error) {
        console.error('Failed to fetch teamspaces:', error);
      } finally {
        setLoading(false);
      }
    };

    fetchTeamspaces();
  }, []);

  if (loading) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: '50vh' }}>
        <Typography>Loading teamspaces...</Typography>
      </Box>
    );
  }

  return (
    <Box>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 4 }}>
        <Typography variant="h4">Teamspaces</Typography>
        <Button component={Link} to="/create" variant="contained" color="primary">
          Create Teamspace
        </Button>
      </Box>

      {teamspaces.length === 0 ? (
        <Typography>No teamspaces found. Create your first teamspace!</Typography>
      ) : (
        <Box sx={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(300px, 1fr))', gap: 2 }}>
          {teamspaces.map((teamspace) => (
            <Card key={teamspace.id}>
              <CardContent>
                <Typography variant="h6">{teamspace.name}</Typography>
                <Typography color="text.secondary">{teamspace.description}</Typography>
              </CardContent>
              <CardActions>
                <Button size="small">View Details</Button>
              </CardActions>
            </Card>
          ))}
        </Box>
      )}
    </Box>
  );
}

export default TeamspaceList; 