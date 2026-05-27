import json
import base64
import re
import sys

FILE = r'C:\Users\Docker\AppData\Local\trae-cli\sessions\6221af46-80fb-4e71-8185-c3bb9aa8f5b8\file-cache\mcp__whistle__getInterceptInfo-1779839551212.md'

with open(FILE, 'r', encoding='utf-8') as f:
    content = f.read()

# Find the JSON array - it starts after the markdown header
idx = content.find('[')
if idx < 0:
    print("ERROR: Could not find JSON array start")
    sys.exit(1)

content = content[idx:]
data = json.loads(content)
print(f"Total records: {len(data)}")

# Show unique URLs
urls = set()
for d in data:
    urls.add(d.get('url', ''))
print("\nUnique URLs:")
for u in sorted(urls):
    print(f"  {u}")

# Count llm_raw_chat
llm_count = sum(1 for d in data if 'llm_raw_chat' in d.get('url', ''))
print(f"\nllm_raw_chat records: {llm_count}")

print("\n" + "="*130)
print("LLM RAW CHAT SUMMARIES")
print("="*130)

def safe_decode_base64(b64_str):
    """Decode base64, return empty string on failure."""
    if not b64_str:
        return ""
    try:
        return base64.b64decode(b64_str).decode('utf-8', errors='replace')
    except Exception:
        return ""

def extract_sse_fields(body_str):
    """Extract fields from SSE response body (token_usage, timing_cost events)."""
    result = {}

    # Extract from token_usage event
    m = re.search(r'event: token_usage\ndata: ({[^\n]+})', body_str)
    if m:
        usage_json = m.group(1)
        for key, pat in [
            ('cache_read', r'"cache_read_input_tokens"\s*:\s*(\d+)'),
            ('cache_creation', r'"cache_creation_input_tokens"\s*:\s*(\d+)'),
            ('prompt_tokens', r'"prompt_tokens"\s*:\s*(\d+)'),
            ('completion_tokens', r'"completion_tokens"\s*:\s*(\d+)'),
            ('total_tokens', r'"total_tokens"\s*:\s*(\d+)'),
        ]:
            m2 = re.search(pat, usage_json)
            if m2:
                result[key] = m2.group(1)

    # Extract from timing_cost event
    m = re.search(r'event: timing_cost\ndata: ({[^\n]+})', body_str)
    if m:
        timing_json = m.group(1)
        for key, pat in [
            ('preprocess_timing', r'"preprocess_timing"\s*:\s*(\d+)'),
            ('postprocess_timing', r'"postprocess_timing"\s*:\s*(\d+)'),
            ('queue_timing', r'"queue_timing"\s*:\s*(\d+)'),
            ('platform_first_token_timing', r'"platform_first_token_timing"\s*:\s*(\d+)'),
            ('server_processing_time', r'"server_processing_time"\s*:\s*(\d+)'),
            ('first_sse_event_time', r'"first_sse_event_time"\s*:\s*(\d+)'),
        ]:
            m2 = re.search(pat, timing_json)
            if m2:
                result[key] = m2.group(1)

    # Extract from metadata event (session_id from server)
    m = re.search(r'event: metadata\ndata: ({[^\n]+})', body_str)
    if m:
        meta_json = m.group(1)
        m2 = re.search(r'"session_id"\s*:\s*"([^"]+)"', meta_json)
        if m2:
            result['server_session_id'] = m2.group(1)

    # Extract from done event
    m = re.search(r'event: done\ndata: ({[^\n]+})', body_str)
    if m:
        done_json = m.group(1)
        m2 = re.search(r'"finish_reason"\s*:\s*"([^"]+)"', done_json)
        if m2:
            result['finish_reason'] = m2.group(1)

    return result

def extract_req_body_fields(body_str):
    """Extract key fields from request body (first 500 chars)."""
    result = {}
    body_preview = body_str[:500] if body_str else ""
    patterns = {
        'config_name': r'"config_name"\s*:\s*"([^"]+)"',
        'model_name': r'"model_name"\s*:\s*"([^"]+)"',
        'conversation_id': r'"conversation_id"\s*:\s*"([^"]+)"',
        'session_id': r'"session_id"\s*:\s*"([^"]+)"',
    }
    for key, pat in patterns.items():
        m = re.search(pat, body_preview)
        if m:
            result[key] = m.group(1)
    return result

for d in data:
    url = d.get('url', '')
    if 'llm_raw_chat' not in url:
        continue

    rid = d.get('id', '?')
    start_time = d.get('startTime', '?')
    ttfb = d.get('ttfb', '?')

    req = d.get('req', {})
    res = d.get('res', {})
    req_size = req.get('size', '?')
    res_status = res.get('statusCode', '?')
    res_size = res.get('size', '?')

    req_headers = req.get('headers', {})
    res_headers = res.get('headers', {})

    # Extra header (first 200 chars)
    extra = req_headers.get('Extra', req_headers.get('extra', ''))
    extra_short = extra[:200] if extra else ''

    # X-Ide-Function
    x_ide_func = req_headers.get('X-Ide-Function', req_headers.get('x-ide-function', ''))

    # Server-Timing
    server_timing = res_headers.get('Server-Timing', res_headers.get('server-timing', ''))

    # Content-Type
    content_type = res_headers.get('Content-Type', res_headers.get('content-type', ''))

    # Decode request body
    req_body_b64 = req.get('base64', '')
    req_body_str = safe_decode_base64(req_body_b64)
    req_fields = extract_req_body_fields(req_body_str)

    # Also extract session_id from Extra header
    if req_fields.get('session_id', '?') == '?' and extra:
        m = re.search(r'"session_id"\s*:\s*"([^"]+)"', extra)
        if m:
            req_fields['session_id'] = m.group(1)

    # Decode response body and extract SSE fields
    res_body_b64 = res.get('base64', '')
    res_body_str = safe_decode_base64(res_body_b64)
    sse_fields = extract_sse_fields(res_body_str)

    # Build summary line
    model = req_fields.get('config_name', req_fields.get('model_name', '?'))
    session = req_fields.get('session_id', '?')
    conv = req_fields.get('conversation_id', '?')
    server_session = sse_fields.get('server_session_id', '')

    cache_read = sse_fields.get('cache_read', '?')
    cache_creation = sse_fields.get('cache_creation', '?')
    prompt = sse_fields.get('prompt_tokens', '?')
    completion = sse_fields.get('completion_tokens', '?')
    total = sse_fields.get('total_tokens', '?')
    first_token = sse_fields.get('platform_first_token_timing', '?')
    preprocess = sse_fields.get('preprocess_timing', '?')
    postprocess = sse_fields.get('postprocess_timing', '?')
    queue_timing = sse_fields.get('queue_timing', '?')
    server_proc = sse_fields.get('server_processing_time', '?')
    first_sse = sse_fields.get('first_sse_event_time', '?')
    finish_reason = sse_fields.get('finish_reason', '?')

    # Get tt_agw from Server-Timing header
    tt_agw = '?'
    if server_timing:
        m = re.search(r'tt_agw;\s*dur=(\d+)', server_timing)
        if m:
            tt_agw = m.group(1)

    # Calculate cache hit rate: cache_read / prompt_tokens * 100
    cache_hit_rate = '?'
    if cache_read != '?' and prompt != '?' and int(prompt) > 0:
        cache_hit_rate = f"{int(cache_read) / int(prompt) * 100:.1f}"

    # Main summary line
    print(f"[{rid}] [{start_time}] TTFB={ttfb}ms | model={model} | session={session} | conv={conv} | req={req_size}B | res={res_status}/{res_size}B | cache_read={cache_read} cache_hit={cache_hit_rate}% | prompt={prompt} completion={completion} total={total} | tt_agw={tt_agw}ms first_token={first_token}ms | finish={finish_reason}")

    # Extra details
    if extra_short:
        print(f"  Extra: {extra_short}")
    if x_ide_func:
        print(f"  X-Ide-Function: {x_ide_func}")
    if server_timing:
        print(f"  Server-Timing: {server_timing}")
    if content_type:
        print(f"  Content-Type: {content_type}")
    if server_session:
        print(f"  server_session_id: {server_session}")
    if preprocess != '?':
        print(f"  preprocess={preprocess}ms postprocess={postprocess}ms queue={queue_timing}ms server_proc={server_proc}ms first_sse={first_sse}ms")
    if cache_creation != '?' and cache_creation != '0':
        print(f"  cache_creation: {cache_creation}")
    print()