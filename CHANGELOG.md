v2.0.0
----------
 * Replace rmq with Redis Streams
 * Refactor client presence
 * Add janitor/cleanup for streams
 * Improve shutdown and messaging

v1.9.3
----------
 * Refactor websocket client by removing ping management
 * Enhance error logging in websocket client

v1.9.2
----------
 * Fix: remove write deadline after sending message

v1.9.1
----------
 * Fix: add root route to avoid 404

v1.9.0
----------
 * Feat: socket improvements
 * Update rmq and cleaner

v1.8.9
----------
 * Tweaks on connections for redis queues

v1.8.8
----------
 * Send ready for message confirmation on register

v1.8.7
----------
 * change queue retention limit from hour to minute

v1.8.6
----------
 * Add client timeout verification

v1.8.5
----------
 * Add typing indicator

v1.8.4
----------
 * Add context to outgoing messages

v1.8.3
----------
 * fix rmq queues leak

v1.8.2
----------
 * Domain verification is only for own host

v1.8.1
----------
 * Dont log entity not found

v1.8.0
----------
 * Add domain restriction option

v1.7.3
----------
 * Fix parse message from channel when empty
 * Healthcheck tweaks to on check db use main app db connection to avoid open new
 * Fix process trigger in proper way to prevent be saved on history

v1.7.2
----------
 * Fix msg timestamps

v1.7.1
----------
 * Allow history for all session types

v1.7.0
----------
 * Refactor client management
 * Refactor message delivery
 * Configs for Healthcheck timeout an ClientTTL
 * Fix racing condition on client management at connection heartbeat
 * Fix mongodb readpref primary, now is set to default

v1.6.10

Added:
- Configs for msgs(keys) expiration as RetentionLimit as 12 hour for default and Redis pool timeout to 15 instead 6

v1.6.9

Added:
- Fix close client queue on exit

v1.6.8

Added:
- Fix close session

v1.6.7

Added:
- Now token comes from client

v1.6.6

Added:
- Fix concurrent write to websocket connection

v1.6.5

Added:
- Add support for multiple instances
- Update codecov action from v1 to v3

v1.6.4

Added:
- Add concurrent safe method for find client on pool

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
