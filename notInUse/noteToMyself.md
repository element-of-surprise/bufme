Notes to myself:

I like bufcli, super useful and I like the devs I've talked to. For various reasons they restrict how useful the CLI is without the BSR, which makes sense to me.  

I did think about just mounting the go packages instead of depending on the CLI, but:

## They disable access

The needed methods for generate are in private/.  Those are protected by a usage.go file that prevents you from using them.  Disabling that is easy, but then there is a bunch of flag and Container type madness. I'm sure there is a reason for the
usage file instead of just doing internal/, not sure what it is.

## Container abstraction and creation madness

Or it might be genius and I'm having trouble understanding.  Everything is wrapped in a bunch of cobra to app, appflags, .... abstractions that make everything hard to reason on the flow.  The main funcs are fine except these app things you gotta pass.  These various Container types that wrap each other provide things like app name, loggers, ... (there are so many different ways they pass the logger, my brain hurts, which seems like a trend in Go packages I keep looking at. I just finished helping undo that for some internal teams.)

## Result

Anyways, going to leave this here.  I might go figure it out and not take a dependency on the actual app in the future. 

But between those abstractions, forking the repo, renaming the imports and looking at funcs that have an args list that spans lines, I'm just going to go back to the easy way. I got book deadlines and new hires, can't be dealing with this at the moment. And for no other reason, the private/ stuff is likely to change, the CLI interface looks more solid.
