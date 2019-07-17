# YourBase CLI 

`yb build all-the-things!`

A build tool that makes working on projects much more delightful. Stop worrying
about dependencies and keep your CI build process in-sync with your local
development process. 

The primary features of the YB tooling are:

* *Consistent local and CI tooling* How a project is built as part of the
  CI/CD process should not be any different than how it is built on a
  developer's machine. Keeping tooling in-sync means predictable results and 
  a better developer experience for everyone. 

* *Accelerated on-boarding* Many projects have long sets of instructions that 
  are required for a developer to get started. With YB, the experience is as 
  simple as getting source code and running `yb build` - batteries included!

* *Programmatic dependency management* No need to have developers manually
  install and manage different versions of Go, Node, Java, etc. By describing
  these tools in codified build-packs, these can be installed and configured 
  automatically on a per-project basis. Manage containers and other runtime 
  dependencies programmatically and in a consistent manner. 

![magic!](http://www.reactiongifs.com/r/mgc.gif)

# How to use it

1. Download and install `yb` from https://dl.equinox.io/yourbase/yb/stable - alternatively, build the code in this repository using `go build` with a recent version of go. 
2. Clone a package from GitHub 
3. Write a simple build manifest (more below)
4. Build awesome things!

# Documentation 

We are working on publishing documentation but for now you can look at the
`.yourbase.yml` file included in this repository or look in the Wiki for some
examples of how to get started using the tool.

# Contributing 

We welcome contributions to this CLI, please see the CONTRIBUTING file for more
information. 

# License 

This project is licensed under the Apache 2.0 license, please see the LICENSE
file for more information.

