#include "stats.h"
#include <string.h>
#include <time.h>

proxy_stats_t g_stats;

void stats_init() {
    memset(&g_stats, 0, sizeof(proxy_stats_t));
    pthread_mutex_init(&g_stats.lock, NULL);
    g_stats.start_time = time(NULL);
}

void stats_record_get() {
    pthread_mutex_lock(&g_stats.lock);
    g_stats.total_requests++;
    g_stats.get_requests++;
    pthread_mutex_unlock(&g_stats.lock);
} 

void stats_record_connect() {
    pthread_mutex_lock(&g_stats.lock);
    g_stats.total_requests++;
    g_stats.connect_requests++;
    pthread_mutex_unlock(&g_stats.lock);
}

void stats_record_error() {
    pthread_mutex_lock(&g_stats.lock);
    g_stats.error_count++;
    pthread_mutex_unlock(&g_stats.lock);
}

void stats_record_bytes_origin(size_t bytes){
    pthread_mutex_lock(&g_stats.lock);
    g_stats.bytes_origin += bytes;
    pthread_mutex_unlock(&g_stats.lock);
}

void stats_record_bytes_cache(size_t bytes){
    pthread_mutex_lock(&g_stats.lock);
    g_stats.bytes_cache += bytes;
    pthread_mutex_unlock(&g_stats.lock);
}