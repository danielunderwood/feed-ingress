with (import <nixpkgs> {});
mkShell {
  buildInputs = [
    go_1_17
    go-tools
    docker-compose
    kcat
  ];
}
