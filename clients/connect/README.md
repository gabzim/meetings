## Connect

This binary allows you to connect to the websocket with the token you get from oauth.

It prints out incoming events to stdout. It will receive the same event multiple times, it's up to you to deduplicate or see if you've already acted on the event you receive.
When an event you've already received is received a second or third time, it may be identical or may have changes (like if it's been cancelled).
