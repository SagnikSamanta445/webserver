#ifndef STATS_H
#define STATS_H

#include <stddef.h>
#include <pthread.h>
#include <time.h>

typedef struct {
    size_t total_requests;
    size_t get_requests;
    size_t connect_requests;
    size_t error_count;
    size_t bytes_origin;
    size_t bytes_cache;
    time_t start_time;
    pthread_mutex_t lock;
} proxy_stats_t;

extern proxy_stats_t g_stats;

void stats_init();
void stats_record_get();
void stats_record_connect();
void stats_record_error();
void stats_record_bytes_origin(size_t bytes);
void stats_record_bytes_cache(size_t bytes);

#endif