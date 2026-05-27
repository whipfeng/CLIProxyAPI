import base64
import json

with open(r'C:\Users\Docker\vs-project\workspace\CLIProxyAPI\tmp\b64_input.txt', 'r') as f:
    b64_data = f.read()

data = base64.b64decode(b64_data)
j = json.loads(data)
msgs = j['messages']
first = msgs[0]
last = msgs[-1]

first_text = first['content'][0]['text']
last_text = last['content'][0]['text']

print(f"API #2 | conv_id={j['conversation_id']} | msgs={len(msgs)} | first_role={first['role']} | first_text={first_text} | last_role={last['role']} | last_text={last_text[:100]}")
