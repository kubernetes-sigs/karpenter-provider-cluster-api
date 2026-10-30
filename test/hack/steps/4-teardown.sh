#!/bin/bash

set -euo pipefail

kind delete cluster --name km-cp
kind delete cluster --name kind
