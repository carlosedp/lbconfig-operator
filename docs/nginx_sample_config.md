# NGINX Configuration as LB

## Using HTTPS

```json
https {
    upstream myapp1 {
        zone   stream_backend 64k;
        server srv1.example.com;
        server srv2.example.com;
        server srv3.example.com;
    }

    server {
        listen 80;

        location / {
            proxy_pass https://myapp1;
            health_check port=8080 uri=/healthz;
        }
    }
}
```

## Using TCP (L4)

```json
stream {
    upstream myservice {
        zone   stream_backend 64k;
        server read1.example.com:6443;
        server read2.example.com:6443;
        server 10.10.12.34:6443;
    }

    match https {
        send      "GET /healthz HTTP/1.0\r\nHost: localhost\r\n\r\n";
        expect ~* "200 OK";
    }

    server {
        listen 3306;
        proxy_pass myservice;
        health_check port=8080 match=https;
    }
}
```
