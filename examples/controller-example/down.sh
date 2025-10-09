#!/bin/bash

kind delete cluster --name "hub" || true
kind delete cluster --name "spoke" || true
