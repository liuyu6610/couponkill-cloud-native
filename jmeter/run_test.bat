@echo off
chcp 65001
echo 正在运行JMeter性能测试...

cd /d "D:\jmeter\apache-jmeter-5.6.3\apache-jmeter-5.6.3\bin"
jmeter.bat -n -t D:\couponkill\couponkill-cloud-native\jmeter\test_plan.jmx -q D:\couponkill\couponkill-cloud-native\jmeter\performance.properties -l D:\couponkill\couponkill-cloud-native\jmeter\test_results.jtl -j D:\couponkill\couponkill-cloud-native\jmeter\jmeter.log

echo.
echo 测试完成！
echo 结果已保存到 D:\couponkill\couponkill-cloud-native\jmeter\test_results.jtl
echo 日志已保存到 D:\couponkill\couponkill-cloud-native\jmeter\jmeter.log
echo.

pause