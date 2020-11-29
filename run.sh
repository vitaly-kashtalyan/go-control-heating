#!/usr/bin/env bash

#docker-compose up --build -d

#unset VARIABLE_NAME

export RELAYS_SERVICE_HOST="192.168.0.8:8082"
export SENSORS_SERVICE_HOST="192.168.0.8:8084"
export RULES_SERVICE_HOST="192.168.0.8:8071"

source ~/.zshrc