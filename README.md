A personal paste bin mimicing the choices of ix.io, now RIP.

https://web.archive.org/web/20231127094241/http://ix.io/

```
USAGE:
- POST /       : Create a new snippet. Send snippet text as the request body.
- GET /{id}    : Retrieve a snippet with the given id.
- PUT /{id}    : Update an existing snippet. Send updated text as the request body.
- DELETE /{id} : Delete a snippet with the given id.

EXAMPLES:
- curl -X POST --data "tomato" http://localhost:8080
- curl http://localhost:8080/1
- curl -X PUT --data "potato" http://localhost:8080/1
- curl -X DELETE http://localhost:8080/1
```
