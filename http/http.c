#include "http.h"
#include "cache.h"
#include "stats.h"
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <arpa/inet.h>
#include <netdb.h>
#include <sys/socket.h>


#define MAX_BYTES 4096

/* ---------------- CONNECT TO REMOTE SERVER ---------------- */

int connectRemoteServer(char* host_addr, int port_num)
{
    struct addrinfo hints, *res;
    memset(&hints, 0, sizeof(hints));
    hints.ai_family   = AF_INET;
    hints.ai_socktype = SOCK_STREAM;

    char port_str[6];
    snprintf(port_str, sizeof(port_str), "%d", port_num);

    if (getaddrinfo(host_addr, port_str, &hints, &res) != 0)
        return -1;

    // Create socket using the result's own fields
    int remoteSocket = socket(res->ai_family,
                              res->ai_socktype,
                              res->ai_protocol);
    if (remoteSocket < 0)
    {
        freeaddrinfo(res);
        return -1;
    }

    if (connect(remoteSocket, res->ai_addr, res->ai_addrlen) < 0)
    {
        freeaddrinfo(res);
        close(remoteSocket);
        return -1;
    }

    freeaddrinfo(res);
    return remoteSocket;
}

/* ---------------- HANDLE REQUEST ---------------- */

int handle_request(int clientSocket,
                   struct ParsedRequest *request,
                   char *tempReq)
{
    char *buf = malloc(MAX_BYTES);
    if (!buf)
        return -1;

    snprintf(buf, MAX_BYTES,
             "GET %s %s\r\n",
             request->path,
             request->version);

    size_t len = strlen(buf);

    ParsedHeader_set(request, "Connection", "close");

    if (ParsedHeader_get(request, "Host") == NULL)
        ParsedHeader_set(request, "Host", request->host);

    if (ParsedRequest_unparse_headers(request,
                                      buf + len,
                                      MAX_BYTES - len) < 0)
    {
        free(buf);
        return -1;
    }

    buf[MAX_BYTES - 1] = '\0';

    int server_port =
        request->port ? atoi(request->port) : 80;

    int remoteSocketID =
        connectRemoteServer(request->host,
                            server_port);

    if (remoteSocketID < 0)
    {
        free(buf);
        return -1;
    }

    send(remoteSocketID, buf, strlen(buf), 0);

    char response[MAX_BYTES];
    int bytes_recv;

    int total_size = 0;
    int capacity = MAX_BYTES;
    char* temp_buffer = malloc(capacity);

    if (!temp_buffer)
    {
        free(buf);
        close(remoteSocketID);
        return -1;
    }

    while ((bytes_recv =
            recv(remoteSocketID,
                 response,
                 MAX_BYTES,
                 0)) > 0)
    {
        send(clientSocket,
             response,
             bytes_recv,
             0);

        if (total_size + bytes_recv > capacity)
        {
            capacity *= 2;
            char* new_buf = realloc(temp_buffer, capacity);
            if(!new_buf){
                free(temp_buffer);
                temp_buffer = NULL;
                break;
            }
            temp_buffer = new_buf;
        }

        memcpy(temp_buffer + total_size,
               response,
               bytes_recv);

        total_size += bytes_recv;
    }

    stats_record_bytes_origin(total_size);
    cache_put(tempReq,
                      temp_buffer,
                      total_size);

    free(temp_buffer);
    free(buf);
    close(remoteSocketID);

    return 0;
}

/* ---------------- ERROR RESPONSE ---------------- */

int sendErrorMessage(int socket, int status_code)
{
    char buffer[512];

    switch(status_code)
    {
        case 400:
            strcpy(buffer,
            "HTTP/1.1 400 Bad Request\r\n\r\n");
            break;

        case 500:
            strcpy(buffer,
            "HTTP/1.1 500 Internal Server Error\r\n\r\n");
            break;

        default:
            strcpy(buffer,
            "HTTP/1.1 500 Internal Server Error\r\n\r\n");
            break;
    }

    send(socket, buffer, strlen(buffer), 0);
    return 1;
}

/* ---------------- VERSION CHECK ---------------- */

int checkHTTPversion(char *msg)
{
    if (!strncmp(msg, "HTTP/1.1", 8))
        return 1;

    if (!strncmp(msg, "HTTP/1.0", 8))
        return 1;

    return -1;
}