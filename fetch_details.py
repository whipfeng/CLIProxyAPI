import json
import urllib.request
import gzip
import io

# Load the main data
with open(r'C:\Users\Docker\vs-project\workspace\CLIProxyAPI\whistle_data.json', 'r', encoding='utf-8') as f:
    data = json.load(f)

recs = data['data']['data']

# Find all llm_raw_chat requests
llm_ids = []
for rid, r in recs.items():
    if 'llm_raw_chat' in r.get('url', ''):
        llm_ids.append(rid)

print(f"Found {len(llm_ids)} llm_raw_chat requests:")
for rid in llm_ids:
    r = recs[rid]
    print(f"  {rid}: {r.get('req',{}).get('ip','?')}:{r.get('req',{}).get('port','?')} req_size={r.get('req',{}).get('size','?')} resp_size={r.get('res',{}).get('size','?')}")

# Now fetch each one individually
for rid in llm_ids:
    print(f"\n{'='*80}")
    print(f"REQUEST: {rid}")
    print(f"{'='*80}")
    
    url = f"http://10.11.61.34:8899/cgi-bin/get-data?ids={rid}"
    try:
        resp = urllib.request.urlopen(url, timeout=30)
        raw = resp.read()
        
        # Try to decompress if gzipped
        try:
            raw = gzip.decompress(raw)
        except:
            pass
        
        detail = json.loads(raw)
        drecs = detail.get('data', {}).get('data', {})
        
        if rid in drecs:
            r = drecs[rid]
            
            # Print Extra header
            req_headers = r.get('req', {}).get('headers', {})
            extra = req_headers.get('extra', 'N/A')
            print(f"\nExtra header: {extra}")
            
            # Print request body (first 3000 chars)
            req_body = r.get('req', {}).get('content', '')
            if req_body:
                try:
                    req_body_decoded = req_body if isinstance(req_body, str) else req_body.decode('utf-8', errors='replace')
                except:
                    req_body_decoded = str(req_body)
                print(f"\nRequest body (first 3000 chars):")
                print(req_body_decoded[:3000])
            
            # Print response body (first 3000 chars)
            resp_body = r.get('res', {}).get('content', '')
            if resp_body:
                try:
                    resp_body_decoded = resp_body if isinstance(resp_body, str) else resp_body.decode('utf-8', errors='replace')
                except:
                    resp_body_decoded = str(resp_body)
                print(f"\nResponse body (first 3000 chars):")
                print(resp_body_decoded[:3000])
        else:
            print(f"  Request {rid} not found in detail response")
            print(f"  Available keys: {list(drecs.keys())[:5]}")
    except Exception as e:
        print(f"  Error fetching {rid}: {e}")
