#!/usr/bin/env bash
set -euo pipefail

source ./config/config

chart_ver=$(grep '^version:' charts/kapparmor/Chart.yaml | awk '{print $2}' | tr -d '"')
app_ver=$(grep '^appVersion:' charts/kapparmor/Chart.yaml | awk '{print $2}' | tr -d '"')

if [[ "$APP_VERSION" != "$app_ver" ]] || [[ "$CHART_VERSION" != "$chart_ver" ]]; then
  echo "❌ Version mismatch detected!"
  echo "  config: APP=$APP_VERSION CHART=$CHART_VERSION"
  echo "  chart.yaml: APP=$app_ver CHART=$chart_ver"
  exit 1
fi

echo "✅ Versions are consistent: App = $APP_VERSION / Chart = $CHART_VERSION"
