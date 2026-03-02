#ifndef HTTP_H
#define HTTP_H

#include "proxy_parse.h"

int handle_request(int clientSocket,
                   struct ParsedRequest *request,
                   char *tempReq);

int sendErrorMessage(int socket, int status_code);

int checkHTTPversion(char *msg);

#endif