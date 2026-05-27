import json
import urllib.request
import gzip
import base64

# Check CLIProxy DeepSeek responses
cliproxy_ids = ['1779805321972-683', '1779806069536-409', '1779806275380-513']

for rid in cliproxy_ids:
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
        res = r.get('res', {})
        resp_base64 = res.get('base64', '')
        
        print(f"\n{'='*60}")
        print(f"ID: {rid}")
        print(f"Response size: {res.get('size', 0)}")
        
        if resp_base64:
            try:
                resp_bytes = base64.b64decode(resp_base64)
                resp_str = resp_bytes.decode('utf-8', errors='replace')
            except:
                resp_str = str(resp_base64)
            
            print(f"Response body (first 3000 chars):")
            print(resp_str[:3000])
            
            if 'token_usage' in resp_str:
                print("\n*** FOUND token_usage ***")
                idx = resp_str.find('token_usage')
                print(resp_str[max(0,idx-100):idx+500])
            else:
                print("\nNo token_usage found")
        else:
            print("No base64 content")
    except Exception as e:
        print(f"Error: {e}")
