# CLI Proxy API

Reverse proxy server providing unified multi-provider compatibility with streaming, thinking reasoning pipeline, authentication, and traffic diagnostics.

## Language

**Client Request**:
The inbound HTTP request sent by an end-user client to the proxy server.
_Avoid_: Inbound payload, user query, downstream request

**Client Response**:
The outbound HTTP response returned by the proxy server to the end-user client.
_Avoid_: Inbound response, downstream response, return payload

**Upstream Request**:
The translated outbound HTTP request sent by the proxy executor to a target AI provider API.
_Avoid_: Backend request, provider request, forward request

**Upstream Response**:
The raw or streaming HTTP response received from the target AI provider API.
_Avoid_: Backend response, provider response

**Traffic Dump**:
A diagnostics mechanism that writes individual raw HTTP wire-format files per request cycle.
_Avoid_: Request log, packet capture, wire trace

**Request ID**:
A unique identifier for a client request cycle, extracted from the client's `X-Request-ID` header if present or generated as an 8-character hex string.
_Avoid_: Trace ID, session ID, correlation ID
