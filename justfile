run:
  go run .

clean:
  rm -rf data
  rm index.txt

test:
  #!/bin/sh
  curl -s -X POST --data "yes" http://localhost:8080 | \
      xargs -I {} curl -s -X PUT --data "no" {} | \
      xargs -I {} curl -s -X DELETE {} && echo "all good"
