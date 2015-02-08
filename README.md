Reverse Proxy + Cache for Minecraft
===

[Blog post here](https://blog.benjojo.co.uk/post/minecraft-reverse-proxy-plus-cache-on-demand)

One of the things that I like to play from time to time is Minecraft, however one of things ( at least with me this is ) is that Minecraft is best played with other people, This however means you have to go through all of the faf (If you don’t want to go with Minecraft realms that is) with setting up the server, installing Java on your server and running this fairly heavy java app on the server.

The major sucker of it, is that it generally eats at least ~1GB of ram, even when its doing *nothing at all* and no one is connected to it.

That’s kind of sucky.

<h2>Introducing mcod</h2>

To “fix” this issue (since it is the only one I think I can fix out of this) I have written a really simple reverse proxy for Minecraft, Infact it can be used for just about and TCP port based game if you ignore some of the other options, but for now I will go into how it can be used for Minecraft.

How it works, is that when there is no one on the server (90%+ of the time in my case) the server is shut down and all that is left is the proxy. When a connection comes in, the server is quickly started up again, and the connection is patched though. Then when the player leaves again, after a short amount of time ( to prevent flapping from causing unneeded server restarts ) the server is shut down and back into idle mode.

This is the default setup of the proxy right now.

After running this for a bit, I started to find the server being started quite a bit without a need and then shutting down after the TTL was over. This turned out to be “banner requests” from clients who had my server in the list (or scanners on the internet looking for servers) and asking for how many players etc where on.

To fix this issue of starting up the server unnessarily, I added “banner caching” now when people asked the server for a “banner” and the actual java server was not running, it would simply give the client the previous response it had seen from the java server. Meaning that I would not need to start the java backend, just to know that there were 0 people online.

Right now the “-h” of the program looks like this:

```
$ ./mcod -h
Usage of ./mcod:
  -backend="localhost:25567": The IP address that the MC server listens on when it's online
  -cachebanner=true: disable this if in the future they change the handshake proto
  -listen=":25565": The port / IP combo you want to listen on
  -strict=false: Only allow requests that pass a set of rules to connect
```

By default it will cache the banners, However. It’s worth noting that if you want to use this for other games, you will want to disable that and also alter the StartServer and StopServer scripts.

In addition, this software currently assumes that it will be ran on its own user. Don’t run this program on the same user as other java apps. Since by default the StopServer script kills al java applications running.

You can find the code / source for mcod here: [https://github.com/benjojo/mcod](https://github.com/benjojo/mcod)
