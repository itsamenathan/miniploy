FROM nginx:1.27-alpine

RUN printf '%s\n' \
  '<!doctype html>' \
  '<html lang="en">' \
  '  <head>' \
  '    <meta charset="utf-8">' \
  '    <meta name="viewport" content="width=device-width, initial-scale=1">' \
  '    <title>Hello from miniploy</title>' \
  '  </head>' \
  '  <body style="font-family: sans-serif; display: grid; min-height: 100vh; place-items: center; margin: 0; background: #07111f; color: #eef6ff;">' \
  '    <main style="text-align: center;">' \
  '      <h1>Hello from miniploy</h1>' \
  '      <p>This custom nginx image was built and deployed by miniploy.</p>' \
  '    </main>' \
  '  </body>' \
  '</html>' \
  > /usr/share/nginx/html/index.html

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- http://127.0.0.1/ >/dev/null || exit 1
