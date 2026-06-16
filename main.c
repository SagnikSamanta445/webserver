#include "server.h"
#include "cache.h"
#include "thread_pool.h"
#include "stats/stats.h"

#include <stdio.h>
#include <stdlib.h>
#include <arpa/inet.h>
#include <unistd.h>
#include <signal.h>

#define WORKER_THREADS 8
#define REPORT_INTERVAL 15

static void format_bytes(size_t bytes, char *buf, size_t buflen)
{
    if      (bytes >= 1024 * 1024)
        snprintf(buf, buflen, "%.1f MB", bytes / (1024.0 * 1024.0));
    else if (bytes >= 1024)
        snprintf(buf, buflen, "%.1f KB", bytes / 1024.0);
    else
        snprintf(buf, buflen, "%zu B", bytes);
}

static void print_stats()
{
    pthread_mutex_lock(&g_stats.lock);
    size_t total    = g_stats.total_requests;
    size_t gets     = g_stats.get_requests;
    size_t connects = g_stats.connect_requests;
    size_t errors   = g_stats.error_count;
    size_t b_origin = g_stats.bytes_origin;
    size_t b_cache  = g_stats.bytes_cache;
    time_t start    = g_stats.start_time;
    pthread_mutex_unlock(&g_stats.lock);

    long long uptime = (long long)(time(NULL) - start);
    double rps       = uptime > 0 ? (double)total / uptime : 0.0;
    double err_rate  = total  > 0 ? 100.0 * errors / total : 0.0;
    double bw_saved  = (b_origin + b_cache) > 0 ? 100.0 * b_cache / (b_origin + b_cache) : 0.0;

    char buf_origin[32], buf_cache[32], buf_sz[32];
    format_bytes(b_origin,           buf_origin, sizeof(buf_origin));
    format_bytes(b_cache,            buf_cache,  sizeof(buf_cache));
    format_bytes(cache_current_size(), buf_sz,   sizeof(buf_sz));

    printf("\n========================================================\n");
    printf("  PROXY STATS  [uptime: %llds | %.2f req/s]\n", uptime, rps);
    printf("========================================================\n");
    printf("  REQUESTS\n");
    printf("    Total: %-6zu  GET: %-6zu  CONNECT: %-6zu\n", total, gets, connects);
    printf("    Errors: %-5zu  Error rate: %.2f%%\n", errors, err_rate);
    printf("\n  CACHE\n");
    printf("    Hit ratio: %.2f%%   Evictions: %zu\n",
           cache_hit_ratio() * 100, cache_eviction_count());
    printf("    Size: %s / 200 MB   Entries: %zu\n",
           buf_sz, cache_entry_count());
    printf("\n  BANDWIDTH\n");
    printf("    From origin: %s   From cache: %s\n", buf_origin, buf_cache);
    printf("    Bandwidth saved: %.2f%%\n", bw_saved);
    printf("========================================================\n");
}

static void* stats_reporter(void* arg)
{
    int interval = *(int*)arg;
    while (1) {
        sleep(interval);
        print_stats();
    }
    return NULL;
}

void handle_sigint(int sig)
{
    printf("\nShutting down proxy...\n");
    print_stats();
    exit(0);
}

int main(int argc, char* argv[])
{
    if (argc != 2) {
        printf("Usage: ./proxy <port>\n");
        return 1;
    }

    int port = atoi(argv[1]);

    /* Initialize systems */
    stats_init();
    cache_init();
    thread_pool_init(WORKER_THREADS);

    int server_socket = create_server_socket(port);
    signal(SIGINT, handle_sigint);

    static int interval = REPORT_INTERVAL;
    pthread_t reporter;
    pthread_create(&reporter, NULL, stats_reporter, &interval);

    while (1)
    {
        struct sockaddr_in client_addr;
        socklen_t client_len = sizeof(client_addr);

        int client_socket =
            accept(server_socket,
                   (struct sockaddr*)&client_addr,
                   &client_len);

        if (client_socket < 0) {
            perror("accept failed");
            continue;
        }

        thread_pool_add_task(client_socket);
    }

    close(server_socket);
    return 0;
}