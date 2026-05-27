const fs = require('fs');
const tasks = [
  { src: 'C:/Users/Docker/vs-project/workspace/Cli-Proxy-API-Management-Center/dist/index.html', dst: 'C:/Users/Docker/Desktop/Workspace/proxy-ai-model/static/management.html', label: 'Frontend' },
];
for (const t of tasks) {
  try {
    fs.copyFileSync(t.src, t.dst);
    const srcSize = fs.statSync(t.src).size;
    const dstSize = fs.statSync(t.dst).size;
    console.log(`${t.label}: ${srcSize} -> ${dstSize} ${srcSize === dstSize ? 'OK' : 'MISMATCH'}`);
  } catch (e) {
    console.error(`${t.label} error:`, e.message);
  }
}