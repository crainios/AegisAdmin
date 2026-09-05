# User access log

[Version française](../user-access-log.md)

Root can inspect authentication and account-security events under **Users >
Access log**. Entries include timestamp, source IP address, login when known,
event type and success or failure. Filters are available for user and event.

Proxy-derived client addresses are trusted only through the configured access
path. User-Agent storage is size-limited. Passwords, TOTP secrets, cookies and
submitted authentication codes are never logged.

Dates use the profile locale: day-first 24-hour formatting in French and
month-first 12-hour formatting in English.
