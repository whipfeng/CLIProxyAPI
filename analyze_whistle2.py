import urllib.request
import json

resp = urllib.request.urlopen('http://10.11.61.34:8899/cgi-bin/get-data')
data = json.loads(resp.read())
recs = data['data']['data']

# Count total vs unique
print(f"Total records: {len(recs)}")

# Check for port reuse
ports = {}
for rid, r in recs.items():
    port = r.get('req', {}).get('port', '?')
    if port not in ports:
        ports[port] = []
    ports[port].append(rid)

ports_with_multi = {k:v for k,v in ports.items() if len(v) > 1}
print(f"\nPorts with multiple requests: {len(ports_with_multi)}")
for port, ids in sorted(ports_with_multi.items()):
    print(f"  Port {port}: {ids}")

# Detailed look at all request headers
print("\n=== All Request Headers ===")
for rid, r in recs.items():
    rh = r.get('req', {}).get('headers', {})
    rn = r.get('req', {}).get('rawHeaderNames', {})
    print(f"\n[{rid}] {r.get('req',{}).get('method','?')} {r.get('url','?')[:60]}")
    for k, v in rh.items():
        if k == 'authorization' or k == 'proxy-authorization':
            print(f"  {k}={v[:50]}...")
        else:
            print(f"  {k}={v}")
    # Check for connection header specifically
    if 'connection' in rh:
        print(f"  *** connection={rh['connection']}")
    if 'connection' in rn:
        print(f"  rawHeaderNames.connection={rn['connection']}")

# Check the earlier session 563 data separately
print("\n=== Fetching session 563 separately ===")
try:
    resp2 = urllib.request.urlopen('http://10.11.61.34:8899/cgi-bin/get-data?ids=1779607520831-563')
    data2 = json.loads(resp2.read())
    if 'data' in data2 and 'data' in data2['data']:
        rec = data2['data']['data'].get('1779607520831-563', {})
        if rec:
            print(f"URL: {rec.get('url')}")
            rq = rec.get('req', {})
            print(f"Method: {rq.get('method')} HTTP/{rq.get('httpVersion')}")
            print(f"Client: {rq.get('ip')}:{rq.get('port')}")
            print(f"Headers:")
            for k, v in rq.get('headers', {}).items():
                if k in ('authorization','proxy-authorization','extra'):
                    print(f"  {k}: {v[:80]}...")
                else:
                    print(f"  {k}: {v}")
            rs = rec.get('res', {})
            print(f"\nResponse status: {rs.get('statusCode')}")
            print(f"Response headers:")
            for k, v in rs.get('headers', {}).items():
                print(f"  {k}: {v}")
            print(f"\nuseH2: {rec.get('useH2')}")
            print(f"startTime: {rec.get('startTime')}")
            print(f"endTime: {rec.get('endTime')}")
            print(f"dnsTime: {rec.get('dnsTime')}")
            print(f"requestTime: {rec.get('requestTime')}")
            print(f"connectTime: {rec.get('connectTime')}")
            print(f"responseTime: {rec.get('responseTime')}")
            print(f"TTFB: {rec.get('ttfb')}")
except Exception as e:
    print(f"Error fetching session 563: {e}")