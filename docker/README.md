mkdir -p data/{mysqldata,redsidata,serverdata/logs/ai-server}

docker compose up -d

docker run -d -p 81:80 --name nginxweb -v ./docker/config/nginx.conf:/etc/nginx/conf.d/default.conf nginx 