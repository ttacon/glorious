unit "local-app" {
  name = "local-app"
  description = "example local app"

  slot "dev-slot" {
    provider {
      type = "bash/local"
      workingDir = "/home/user/code/app"
      cmd = "npm run start"
    }

    resolver {
      type = "default"
    }
  }

  slot "image-slot" {
    provider {
      type = "docker/local"
      image = "my-docker-hub/example-app:latest"
    }

    resolver {
      type = "keyword/value"
      keyword = "services/app/local/dev-mode"
      value = "false"
    }
  }
}

unit "haproxy" {
     name = "proxy"
     description = "web proxy for applications"

     slot "image" {
          provider {
              type = "docker/local"
              image = "haproxy:2.0"
          }
     }
}

unit "redis" {
  name = "redis"
  description = "cuz redis"

  slot "image" {
    provider {
      type = "docker/local"
      image = "redis:5.0"

      ports = [
        "16379:6379"
      ]
    }
  }
}
