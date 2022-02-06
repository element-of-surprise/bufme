# Bufme
A tool for compiling protos with full directory paths and cross repo compiles.

![Buffalo](https://static.wikia.nocookie.net/adoptme/images/6/6f/Mega_Neon_Buffalo_%28gif%29.gif/revision/latest/scale-to-width-down/368?cb=20210607084341)

## Introduction

Protocol buffers rock, but protoc should die in a fire. Buf CLI (http://buf.build)has come along to save the day, but it only works the way it wants to work.

That is fine, but I require the use of full import paths and I don't care about modules. I need it to compile across private repos.

Now, their system makes more sense for everyone out there.  You make modules and push them to BSR. You reference the module in BSR that you want to import and the module can have only one file with a proto name per module.  It also makes them some money with the BSR and I think that's great.

Problem is, I'm not there. The BSR isn't an option atm.  Renaming my imports isn't an option, as I have to support other orgs with Visual Studio plugins + others, and they are not moving to buf.  So, we get bufme.

## A few notes:

- Works only for generating Go proto files
- There is absolutely no support, bug fixes, or anything
- This is hacky hack hack hacky.  I mean, come on
- It does work, however..... for me

I could make it work with protoc, but again, it should die in a fire.  Frankly, it was made for Google and Blaze.  It makes no sense in the regular world where the monorepo and Blaze doesn't exist.

Buf has redone most of the proto compiling internally and so their tools is superior in almost everyway.  If you aren't constrained like me, just use their stuff the way it wants.

## HowTo
Here is what you need:

rootDir/
  repoA/
  repoB/
  repoC/
  bufme.config

* rootDir is a directory holding all the repos you need.
* repo[A,B,C]/ are individual git repos
* bufme.config holds the config for this group of repos

bufme.config is a JSON file that contains `{"Root":"/full/path/to/rootDir"}`

You put bufme.config at the root of the path. You can have multiple bufme.config. If bufme.config is actually at the root, the "Root" can be set to `"."`. If you only have one of these rootDir, you can simply have a global config in your home directory. You must specify the full path to rootDir in that case.

Finally, you must have the `buf` tool installed in your path. On linux you could do:

```bash
wget https://github.com/bufbuild/buf/releases/download/v1.0.0-rc12/buf-Linux-x86_64
sudo mv buf-Linux-x86_64 /usr/local/bin/buf
```

To install bufme, if you have Go installed do:
```bash
go install github.com/element-of-surprise/bufme@latest
```

To run the tool, go to a file path under rootDir containing a .proto file.  Run `bufme`. This should generate all `pb.go` files and `grpc_pb.go` files required for that proto.
