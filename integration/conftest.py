import os
import subprocess
import time

import pytest


@pytest.fixture(scope="module")
def tmk_application(request):
    enabled = set(getattr(request, "param", ("http", "mqtt", "modbus")))

    env = os.environ.copy()

    # HTTP controller
    env["TMK_CONTROLLERS_HTTP_ENABLED"] = "true" if "http" in enabled else "false"
    env["TMK_CONTROLLERS_HTTP_ADDR"] = ":8080"

    # MQTT controller
    env["TMK_CONTROLLERS_MQTT_ENABLED"] = "true" if "mqtt" in enabled else "false"
    env["TMK_CONTROLLERS_MQTT_ADDR"] = "tcp://localhost:1883"

    # Modbus controller
    env["TMK_CONTROLLERS_MODBUS_ENABLED"] = "true" if "modbus" in enabled else "false"
    env["TMK_CONTROLLERS_MODBUS_ADDR"] = "0.0.0.0:1502"
    env["TMK_CONTROLLERS_MODBUS_UNIT_ID"] = "4"

    # Regulator settings
    env["TMK_REGULATOR_MODE_CHANGE_HYSTERESIS"] = "2.0"
    env["TMK_REGULATOR_TARGET_HYSTERESIS"] = "1.0"
    process = subprocess.Popen(
        ["./.bin/thermocktat"], env=env, stdout=None, stderr=None
    )
    time.sleep(1)
    yield  # Tests will run after this point

    process.terminate()
    try:
        process.wait(timeout=5)
    except subprocess.TimeoutExpired:
        process.kill()
        process.wait()
