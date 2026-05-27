import json
import urllib.request
import gzip
import base64

# Check request body for a few DeepSeek requests
ids_to_check = [
    '1779672798413-567',  # real client DeepSeek
    '1779805321972-683',  # CLIProxy DeepSeek
    '1779753791333-278',  # real client with token_usage
    '1779778870812-972',  # real client with token_usage
]

for rid in ids_to_check:
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
            print(f"{rid}: NOT FOUND")
            continue
        
        r = drecs[rid]
        req = r.get('req', {})
        req_base64 = req.get('base64', '')
        
        print(f"\n{'='*60}")
        print(f"ID: {rid}")
        
        if req_base64:
            try:
                req_bytes = base64.b64decode(req_base64)
                req_str = req_bytes.decode('utf-8', errors='replace')
            except:
                req_str = str(req_base64)
            
            # Try to parse as JSON and extract model
            try:
                req_json = json.loads(req_str)
                print(f"Request model field: {req_json.get('model', 'N/A')}")
                print(f"Request messages count: {len(req_json.get('messages', []))}")
                # Print first message
                msgs = req_json.get('messages', [])
                if msgs:
                    first = msgs[0]
                    print(f"First message role: {first.get('role', 'N/A')}")
                    content = first.get('content', '')
                    if isinstance(content, str):
                        print(f"First message content (first 200): {content[:200]}")
                    elif isinstance(content, list):
                        print(f"First message content type: list, len={len(content)}")
            except:
                print(f"Request body (first 500): {req_str[:500]}")
        else:
            print("No request base64 content")
    except Exception as e:
        print(f"Error: {e}")
