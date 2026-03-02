#include "server.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <arpa/inet.h>
#include <sys/socket.h>

int create_server_socket(int port)
{
    int server_socket =
        socket(AF_INET, SOCK_STREAM, 0);

    if (server_socket < 0) {
        perror("socket failed");
        exit(1);
    }

    int reuse = 1;
    if (setsockopt(server_socket,
                   SOL_SOCKET,
                   SO_REUSEADDR,
                   &reuse,
                   sizeof(reuse)) < 0)
    {
        perror("setsockopt failed");
        exit(1);
    }

    struct sockaddr_in server_addr;
    memset(&server_addr, 0, sizeof(server_addr));

    server_addr.sin_family = AF_INET;
    server_addr.sin_port = htons(port);
    server_addr.sin_addr.s_addr = INADDR_ANY;

    if (bind(server_socket,
             (struct sockaddr*)&server_addr,
             sizeof(server_addr)) < 0)
    {
        perror("bind failed");
        exit(1);
    }

    if (listen(server_socket, 400) < 0)
    {
        perror("listen failed");
        exit(1);
    }

    printf("Proxy listening on port %d\n", port);

    return server_socket;
}