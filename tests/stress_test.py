import requests
from concurrent.futures import ThreadPoolExecutor, as_completed

proxy = {
    "http": "http://127.0.0.1:8080",
    "https": "http://127.0.0.1:8080",
}

urls = [
    "http://example.com",
    "http://http.badssl.com",
    "https://www.reddit.com",
    "https://www.python.org",
]

tasks = []
for url in urls:
    tasks.extend([url] * 10)

def fetch(url):
    try:
        r = requests.get(
            url,
            proxies=proxy,
            timeout=60,
            verify=False,
            allow_redirects=True,
        )
        return True, f"[OK] {url} -> {r.status_code}"
    except Exception as e:
        return False, f"[FAIL] {url} -> {e}"

success = 0
failure = 0

with ThreadPoolExecutor(max_workers=8) as executor:
    futures = [executor.submit(fetch, url) for url in tasks]

    for future in as_completed(futures):
        ok, msg = future.result()
        print(msg)

        if ok:
            success += 1
        else:
            failure += 1

print("\n========== SUMMARY ==========")
print(f"Success: {success}")
print(f"Failure: {failure}")
print(f"Total  : {success + failure}")