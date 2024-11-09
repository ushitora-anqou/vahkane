.PHONY: run
run:
	OCAMLRUNPARAM=b dune exec bin/main.exe
#	OCAMLRUNPARAM=b EIO_BACKEND=posix dune exec bin/main.exe

.PHONY: build
build:
	dune build

.PHONY: setup
setup:
	opam install . --deps-only --with-test --with-dev-setup
