# syncer

Pushes report files to the sync service. `syncer.upload.upload(path, token, transport)` reads the file and hands it to the transport; `syncer.transport` holds the HTTP transport used in production and the `TransportError` it raises.

Logging goes through the standard `logging` module under the `syncer` logger; in production the handler ships every record to the hosted log service.

Run the tests with `python3 -m unittest discover -s tests -t .`.
