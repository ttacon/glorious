glorious
========
because we shouldn't always be building and running code


glorious is a process manages. glorious itself runs a unit.


## units
A unit is the unit of functionality that glorious considers. Think of this as a
service in supervisord terms. A unit in glorious, though, can have more than one
slot. This is where the fun begins.

## slots
A slot is the literal set of commands that glorious will run.

## units + slots
Together, units add logic to determine which slot to run. This allows us
dynamically change what slot we're running based on system conditions.

An example of where this is useful is if you need to develop code against
a larger system, but don't want to run the development code yourself. At
Mixmax, we often have docker containers that we pull down and run instead
of running our gulp based dev process, so that we only need to run the necessary
dev processes when essential.


