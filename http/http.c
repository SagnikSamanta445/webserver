#include "http.h"
#include "cache.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <arpa/inet.h>
#include <netdb.h>
#include <sys/socket.h>

#define MAX_BYTES 4096

/* ---------------- CONNECT TO REMOTE SERVER ---------------- */

static int connectRemoteServer(char* host_addr, int port_num)
{
    int remoteSocket = socket(AF_INET, SOCK_STREAM, 0);
    if (remoteSocket < 0)
        return -1;

    struct hostent* host = gethostbyname(host_addr);
    if (host == NULL)
        return -1;

    struct sockaddr_in server_addr;
    memset(&server_addr, 0, sizeof(server_addr));

    server_addr.sin_family = AF_INET;
    server_addr.sin_port = htons(port_num);

    memcpy(&server_addr.sin_addr.s_addr,
           host->h_addr_list[0],
           host->h_length);

    if (connect(remoteSocket,
                (struct sockaddr*)&server_addr,
                sizeof(server_addr)) < 0)
    {
        close(remoteSocket);
        return -1;
    }

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

    /* Force null termination */
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
            temp_buffer =
                realloc(temp_buffer, capacity);
            if (!temp_buffer)
                break;
        }

        memcpy(temp_buffer + total_size,
               response,
               bytes_recv);

        total_size += bytes_recv;
    }

    add_cache_element(temp_buffer,
                      total_size,
                      tempReq);

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