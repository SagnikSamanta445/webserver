# Multi-Threaded Web Proxy Server with LRU Cache
A HTTP proxy server implemented in C, designed to handle multiple concurrent client connections using a thread pool and optimize response times through a Least Recently Used (LRU) caching mechanism.

# Purpose
The primary goal of this project is to bridge the gap between a client (e.g., a web browser) and a remote server. By acting as an intermediary, this proxy:

Concurrency: Manages multiple simultaneous requests using a fixed-size thread pool to prevent system resource exhaustion.

Efficiency: Reduces latency and bandwidth usage by caching frequently requested web content.

Protocol Handling: Supports standard GET requests for web browsing and CONNECT tunneling for encrypted/HTTPS traffic.
