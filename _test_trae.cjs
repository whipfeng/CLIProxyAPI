const http = require('http');

const data = JSON.stringify({
  baseUrl: 'https://console.enterprise.trae.cn',
  apiKey: '',
  refreshToken: 'trae-lt-4c2508e625dd11126d2ce1db046421bc126b7db7bbf8de0ba3eec10d',
  configName: 'deepseek-V4-Pro',
  modelName: 'deepseek-V4-Pro__v2'
});

const req = http.request('http://localhost:8317/v0/management/trae-api-key/test', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'X-Management-Key': 'CLI77585*1',
    'Content-Length': Buffer.byteLength(data)
  }
}, res => {
  let body = '';
  res.on('data', c => body += c);
  res.on('end', () => console.log(res.statusCode, body));
});

req.on('error', e => console.error('Error:', e.message));
req.write(data);
req.end();