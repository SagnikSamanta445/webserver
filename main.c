#include "server.h"
#include "cache.h"
#include "thread_pool.h"

#include <stdio.h>
#include <stdlib.h>
#include <arpa/inet.h>
#include <unistd.h>

#define WORKER_THREADS 8

int main(int argc, char* argv[])
{
    if (argc != 2) {
        printf("Usage: ./proxy <port>\n");
        return 1;
    }

    int port = atoi(argv[1]);

    /* Initialize systems */
    cache_init();
    thread_pool_init(WORKER_THREADS);

    int server_socket = create_server_socket(port);

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