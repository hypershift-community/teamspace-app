import axios from 'axios';

// Create an Axios instance with default config
const api = axios.create({
  baseURL: '', // Use relative URLs instead of hardcoding
  withCredentials: true, // Enable sending cookies with cross-origin requests
  headers: {
    'Content-Type': 'application/json',
  },
});

// Add response interceptor for logging
api.interceptors.response.use(
  response => {
    console.log(`API Response [${response.status}]: ${response.config.method?.toUpperCase()} ${response.config.url}`, response.data);
    return response;
  },
  error => {
    console.error('API Error:', error.response || error.message || error);
    return Promise.reject(error);
  }
);

// Add request interceptor for logging
api.interceptors.request.use(
  config => {
    console.log(`API Request: ${config.method?.toUpperCase()} ${config.url}`, config.data || {});
    return config;
  },
  error => {
    console.error('API Request Error:', error);
    return Promise.reject(error);
  }
);

export default api; 