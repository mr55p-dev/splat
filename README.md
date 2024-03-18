# splat

Splat is a single-host container deployment tool.

## Goals

1. Run on a server and start/stop container images (front for docker engine).
   - Host an application and its nginx configuration
2. Expose a cli for starting/stopping applications.
3. Expose a web ui for the same
4. Listen for webhook calls which can trigger deployments
