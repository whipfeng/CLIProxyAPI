import json
import urllib.request
import gzip

# Load the main data
with open(r'C:\Users\Docker\vs-project\workspace\CLIProxyAPI\whistle_data.json', 'r', encoding='utf-8') as f:
    data = json.load(f)

recs = data['data']['data']

# Check a few DeepSeek responses for content
deepseek_ids = [
    '1779672798413-567',  # real client
    '1779805321972-683',  # CLIProxy
    '1779806069536-409',  # CLIProxy
    '1779806275380-513',  # CLIProxy
]

for rid in deepseek_ids:
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
        resp_body = r.get('res', {}).get('content', '')
        resp_size = r.get('res', {}).get('size', 0)
        
        print(f"\n{'='*60}")
        print(f"ID: {rid}")
        print(f"Response size: {resp_size}")
        print(f"Response content length: {len(resp_body) if resp_body else 0}")
        
        if resp_body:
            try:
                resp_str = resp_body if isinstance(resp_body, str) else resp_body.decode('utf-8', errors='replace')
            except:
                resp_str = str(resp_body)
            
            # Print first 2000 chars
            print(f"Response body (first 2000 chars):")
            print(resp_str[:2000])
            
            # Search for token_usage
            if 'token_usage' in resp_str:
                print("\n*** FOUND token_usage ***")
                idx = resp_str.find('token_usage')
                print(resp_str[max(0,idx-50):idx+500])
            else:
                print("\nNo token_usage found in response")
        else:
            print("Response body is EMPTY")
    except Exception as e:
        print(f"Error: {e}")
