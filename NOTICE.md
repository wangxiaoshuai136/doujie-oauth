# NOTICE

OMC Doubao OAuth is an independent, unofficial integration. It is not
affiliated with, endorsed by, or sponsored by ByteDance.

"Doubao", "豆包" and related marks are trademarks of ByteDance or its
affiliates. This project only references the public web/desktop product by
name to describe interoperability.

## Protocol research

The request and event shapes implemented here were derived from observing the
public Doubao web/desktop clients and from the open-source project
[doubao2api](https://github.com/wangchuxiaoji-oss/doubao2api) (Apache-2.0),
which documents the QR login flow and the `/samantha/chat/completion`
protocol. No source code from that project is copied into this repository; the
Go implementation is an independent rewrite.

## No vendor assets

The repository ships only an original logo. It does not redistribute Doubao
client binaries, fonts, images, or other vendor assets.
