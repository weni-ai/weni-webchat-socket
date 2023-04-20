v1.6.3

Added:
- Remove aws check from healthcheck

v1.6.2

Added:
- Add timeout and retries configs for redis
- Improve healthcheck with checks for redis and mongodb and json return with details.

v1.6.1

Added:
- Add timeout config from env var or default, for db operations contexts.
- Add s3 related errors logs.

v1.6.0

Added:
- save and retrieve `message history` with pagination for specific session type.

Removed:
- Remove S3 region from URL

v1.5.4

Added:
- Add more errors code to be ignored.

v1.5.3

Added:
- Ignore log error from low level connection close error, to avoid spam logs and sentry.

v1.5.2

Added:
- Publish message deliveries in queue with expiration time

v1.5.1

Fix:
- Ignore wrong protocol connection  attempts in websocket endpoint
- Ignore "use of closed network connection" by concurrent read write benign error
  
v1.5.0

Added:
- Send token feature for session verification

v1.4.0

Added:
- Workflow creation (CI)
- Add Sentry support

v1.3.1

Fix:
- Error Handling on upgrade connection from http methods

v1.3.0

Added:
- Metrics with prometheus for open connections, client messages, client registrations

v1.2.0

Added:
- Queue System to cache not delivered messages and deliver when able to

v1.1.0

Added:
- Send pong type msg to client on receive ping 

v1.0.0

Added:
- First release v1.0.0
