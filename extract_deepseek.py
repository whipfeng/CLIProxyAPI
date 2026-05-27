import json
import urllib.request
import gzip

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

# Now fetch each one individually and extract Extra + token_usage
deepseek_real = []
deepseek_cliproxy = []
token_usage_results = []

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
        req_headers = r.get('req', {}).get('headers', {})
        extra = req_headers.get('extra', '')
        
        if not extra:
            continue
        
        try:
            extra_json = json.loads(extra)
        except:
            continue
        
        model_name = extra_json.get('model_name', '')
        config_name = extra_json.get('config_name', '')
        display_name = extra_json.get('display_name', '')
        api_host = extra_json.get('api_host', '')
        
        is_deepseek = 'deepseek' in model_name.lower() or 'deepseek' in config_name.lower() or 'deepseek' in display_name.lower()
        
        if not is_deepseek:
            continue
        
        # Determine if real client or CLIProxy
        is_cliproxy = 'api.enterprise.trae.cn' in api_host
        
        entry = {
            'id': rid,
            'model_name': model_name,
            'config_name': config_name,
            'display_name': display_name,
            'api_host': api_host,
            'session_id': extra_json.get('session_id', ''),
            'agent_loop_id': extra_json.get('agent_loop_id', ''),
            'extra': extra,
        }
        
        if is_cliproxy:
            deepseek_cliproxy.append(entry)
        else:
            deepseek_real.append(entry)
        
        # Now check response body for token_usage
        resp_body = r.get('res', {}).get('content', '')
        if resp_body:
            try:
                resp_str = resp_body if isinstance(resp_body, str) else resp_body.decode('utf-8', errors='replace')
            except:
                resp_str = str(resp_body)
            
            if 'token_usage' in resp_str:
                # Find all token_usage events
                lines = resp_str.split('\n')
                for line in lines:
                    if 'token_usage' in line:
                        token_usage_results.append({
                            'request_id': rid,
                            'model_name': model_name,
                            'api_host': api_host,
                            'line': line.strip()[:500]
                        })
    except Exception as e:
        pass

print(f"\n{'='*80}")
print(f"DeepSeek Real Client Requests (console.enterprise.trae.cn): {len(deepseek_real)}")
print(f"{'='*80}")
for entry in deepseek_real:
    print(f"\n  ID: {entry['id']}")
    print(f"  model_name: {entry['model_name']}")
    print(f"  config_name: {entry['config_name']}")
    print(f"  display_name: {entry['display_name']}")
    print(f"  api_host: {entry['api_host']}")
    print(f"  session_id: {entry['session_id']}")
    print(f"  agent_loop_id: {entry['agent_loop_id']}")
    print(f"  Extra JSON: {entry['extra']}")

print(f"\n{'='*80}")
print(f"DeepSeek CLIProxy Requests (api.enterprise.trae.cn): {len(deepseek_cliproxy)}")
print(f"{'='*80}")
for entry in deepseek_cliproxy:
    print(f"\n  ID: {entry['id']}")
    print(f"  model_name: {entry['model_name']}")
    print(f"  config_name: {entry['config_name']}")
    print(f"  display_name: {entry['display_name']}")
    print(f"  api_host: {entry['api_host']}")
    print(f"  session_id: {entry['session_id']}")
    print(f"  agent_loop_id: {entry['agent_loop_id']}")
    print(f"  Extra JSON: {entry['extra']}")

print(f"\n{'='*80}")
print(f"Token Usage Events: {len(token_usage_results)}")
print(f"{'='*80}")
for tu in token_usage_results:
    print(f"\n  Request: {tu['request_id']}")
    print(f"  Model: {tu['model_name']}")
    print(f"  API Host: {tu['api_host']}")
    print(f"  Line: {tu['line']}")
