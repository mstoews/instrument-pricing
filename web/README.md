# Pricing workstation

Angular 22 + NgRx Signal Store. See ../README.md for full platform setup.

```sh
npm ci
npm start
npm test -- --watch=false
npm run build
```

Go must run on 127.0.0.1:8080. The dev server proxies /v1 requests. Open http://127.0.0.1:4200 and select Apple Inc. (sample).

State flow: components → store methods → HTTP services → Go REST. Stores own server data and request status; components read signals. Market vendor adapters remain in Python.
