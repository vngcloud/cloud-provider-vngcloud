# vngcloud-controller-manager

![dev-staging-env](https://badgen.net/badge/DEV-STAGING/environment/blue?icon=github)

## Build and push image

- For short, just run the following command:

  ```bash=
  make clean && push-multiarch-images
  ```
  
- Quick build:

  ```bash=
  make clean && make build && make bush-local-images
  ```
