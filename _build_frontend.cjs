const { execSync } = require('child_process');
try {
  execSync('npm run build', {
    cwd: 'C:\\Users\\Docker\\vs-project\\workspace\\Cli-Proxy-API-Management-Center',
    stdio: 'inherit'
  });
  console.log('Build succeeded');
} catch (e) {
  console.error('Build failed:', e.message);
}