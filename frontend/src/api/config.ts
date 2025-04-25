import axios from 'axios';

const api = axios.create({
  baseURL: '/api',
  withCredentials: true, // Important for session cookies
});

export default api; 