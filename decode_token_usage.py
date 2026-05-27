import json
import urllib.request
import gzip
import base64

# Load the main data
with open(r'C:\Users\Docker\vs-project\workspace\CLIProxyAPI\whistle_data.json', 'r', encoding='utf-8') as f:
    data = json.load(f)

recs = data['data']['data']

# Find all llm_raw_chat requests
llm_ids = []
for rid, r in recs.items():
    if 'llm_raw_chat' in r.get('url', ''):
        llm_ids.append(rid)

print(f"Total llm_raw_chat requests: {len(llm_ids)}")

deepseek_real_token_usage = []
deepseek_cliproxy_token_usage = []

for rid in llm_ids:
    url = f"http://10.11.61.34:8899/cgi-bin/get-data?ids={rid}"
    try:
        resp = urllib.request.urlopen(url, timeout=30)
        raw = resp.read()
        try:
            raw = gzip.decompress(raw)
        except:
            pass
        
        detail = json.loads(raw)
        drecs = detail.get('data', {}).get('data', {})
        
        if rid not in drecs:
            continue
        
        r = drecs[rid]
        
        # Get Extra header
        req_headers = r.get('req', {}).get('headers', {})
        extra = req_headers.get('extra', '')
        if not extra:
            continue
        
        try:
            extra_json = json.loads(extra)
        except:
            continue
        
        model_name = extra_json.get('model_name', '')
        is_deepseek = 'deepseek' in model_name.lower()
        if not is_deepseek:
            continue
        
        api_host = extra_json.get('api_host', '')
        is_cliproxy = 'api.enterprise.trae.cn' in api_host
        
        # Decode response body from base64
        res = r.get('res', {})
        resp_base64 = res.get('base64', '')
        if not resp_base64:
            continue
        
        try:
            resp_bytes = base64.b64decode(resp_base64)
            resp_str = resp_bytes.decode('utf-8', errors='replace')
        except:
            continue
        
        if 'token_usage' not in resp_str:
            continue
        
        # Find all token_usage events
        lines = resp_str.split('\n')
        for i, line in enumerate(lines):
            if 'token_usage' in line:
                # Get surrounding context
                start = max(0, i-2)
                end = min(len(lines), i+3)
                context = '\n'.join(lines[start:end])
                
                entry = {
                    'request_id': rid,
                    'model_name': model_name,
                    'api_host': api_host,
                    'is_cliproxy': is_cliproxy,
                    'line': line.strip(),
                    'context': context,
                }
                
                if is_cliproxy:
                    deepseek_cliproxy_token_usage.append(entry)
                else:
                    deepseek_real_token_usage.append(entry)
    except Exception as e:
        pass

print(f"\n{'='*80}")
print(f"DeepSeek Real Client Token Usage Events: {len(deepseek_real_token_usage)}")
print(f"{'='*80}")
for tu in deepseek_real_token_usage:
    print(f"\n  Request: {tu['request_id']}")
    print(f"  Model: {tu['model_name']}")
    print(f"  API Host: {tu['api_host']}")
    print(f"  Context:\n{tu['context']}")

print(f"\n{'='*80}")
print(f"DeepSeek CLIProxy Token Usage Events: {len(deepseek_cliproxy_token_usage)}")
print(f"{'='*80}")
for tu in deepseek_cliproxy_token_usage:
    print(f"\n  Request: {tu['request_id']}")
    print(f"  Model: {tu['model_name']}")
    print(f"  API Host: {tu['api_host']}")
    print(f"  Context:\n{tu['context']}")
