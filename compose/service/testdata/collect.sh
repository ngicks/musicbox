#! /bin/bash
docker compose -f compose.yml -f additional.yml down -v --remove-orphans
echo "FOO=BAR" > test.env
docker compose -f compose.yml -f additional.yml --progress plain --dry-run create 2> 00_create-dryrun.txt
docker compose -f compose.yml -f additional.yml --progress plain create 2> 01_create.txt
docker compose -f compose.yml -f additional.yml --progress plain start 2> 02_start.txt
echo "FOO=BAZ" > test.env
docker compose --progress plain --dry-run create --remove-orphans 2> 03_recreate-dryrun.txt
docker compose --progress plain create --remove-orphans 2> 04_recreate.txt
docker compose --progress plain --dry-run start 2> 05_restart-dryrun.txt
docker compose --progress plain start 2> 06_restart.txt
docker compose --progress plain down -v 2> 07_down.txt
docker compose --progress plain -f nonexistent.yml create 2> 08_nonexistent_compose_yml.txt