with (import <nixpkgs> {});
mkShell {
  buildInputs = [
    go
    docker-compose
    kcat
  ];
}
