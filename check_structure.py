import json
import urllib.request
import gzip

rid = '1779805321972-683'  # CLIProxy DeepSeek
url = f"http://10.11.61.34:8899/cgi-bin/get-data?ids={rid}"
resp = urllib.request.urlopen(url, timeout=30)
raw = resp.read()
try:
    raw = gzip.decompress(raw)
except:
    pass

detail = json.loads(raw)
drecs = detail.get('data', {}).get('data', {})
r = drecs[rid]

# Print all keys in the response
print("Top-level keys:", list(r.keys()))
print("\nreq keys:", list(r.get('req', {}).keys()))
print("\nres keys:", list(r.get('res', {}).keys()))

# Check res structure
res = r.get('res', {})
for k, v in res.items():
    if k == 'headers':
        print(f"\nres.{k}: {json.dumps(v, indent=2)[:500]}")
    elif k == 'content':
        print(f"\nres.{k}: type={type(v).__name__}, len={len(v) if v else 0}")
    else:
        print(f"\nres.{k}: {str(v)[:200]}")

# Check if there's a body field
print("\n\nLooking for body/responseBody fields...")
for k in r.keys():
    if 'body' in k.lower():
        print(f"  Found: {k}")

# Check req content
req = r.get('req', {})
req_content = req.get('content', '')
print(f"\nreq.content: type={type(req_content).__name__}, len={len(req_content) if req_content else 0}")
if req_content:
    try:
        req_str = req_content if isinstance(req_content, str) else req_content.decode('utf-8', errors='replace')
    except:
        req_str = str(req_content)
    print(f"req.content (first 500): {req_str[:500]}")
