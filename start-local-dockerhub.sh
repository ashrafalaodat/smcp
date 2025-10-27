docker run -d \
  --restart=always \
  -p 5000:5000 \
  --name local-registry \
  registry
