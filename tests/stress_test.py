import requests

proxy = {
    "http": "http://127.0.0.1:8080",
}

urls = [
    "http://badssl.com",
    "http://example.com",
    "https://www.reddit.com"
]

for url in urls:
    for i in range(50):
        try:
            r = requests.get(
                url,
                proxies=proxy,
                timeout=5
            )
            print(url, r.status_code)
        except Exception as e:
            print(url, e)