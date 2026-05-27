var fs = require('fs');
var src = 'C:/Users/Docker/vs-project/workspace/Cli-Proxy-API-Management-Center/dist/index.html';
var dst = 'C:/Users/Docker/Desktop/Workspace/proxy-ai-model/static/management.html';
fs.copyFileSync(src, dst);
var s = fs.statSync(src);
var d = fs.statSync(dst);
console.log('SRC:', s.size, 'DST:', d.size, 'OK:', s.size === d.size);