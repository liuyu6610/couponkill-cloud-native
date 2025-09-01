@echo off
set JMETER_HOME=D:\jmeter\apache-jmeter-5.6.3\apache-jmeter-5.6.3
set TEST_PLAN=D:\couponkill\couponkill-cloud-native\jmeter\test_plan.jmx
set RESULTS_FILE=D:\couponkill\couponkill-cloud-native\jmeter\test_results.jtl
set PERF_PROPS=D:\couponkill\couponkill-cloud-native\jmeter\performance.properties
set LOG_FILE=D:\couponkill\couponkill-cloud-native\jmeter\jmeter.log

echo Starting JMeter Performance Test...
echo Test Plan: %TEST_PLAN%
echo Results File: %RESULTS_FILE%
echo Log File: %LOG_FILE%

cd /d %JMETER_HOME%\bin
jmeter -n -t %TEST_PLAN% -q %PERF_PROPS% -l %RESULTS_FILE% -j %LOG_FILE%

echo Test completed. Check results in %RESULTS_FILE%
pause