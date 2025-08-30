@echo off
echo CouponKill秒杀系统JMeter性能测试脚本
echo ======================================

REM 检查JMeter是否已安装
where jmeter >nul 2>&1
if %errorlevel% neq 0 (
    echo 错误: 未找到JMeter命令，请确保JMeter已正确安装并添加到PATH环境变量中
    pause
    exit /b 1
)

REM 设置测试参数
set JMETER_HOME=
set TEST_PLAN=seckill_test_plan.jmx
set RESULTS_FILE=seckill_test_results.jtl
set REPORT_DIR=report

echo 开始执行性能测试...

REM 执行JMeter测试
jmeter -n -t %TEST_PLAN% -l %RESULTS_FILE% -j jmeter.log

if %errorlevel% equ 0 (
    echo 测试执行完成，正在生成测试报告...
    
    REM 创建报告目录
    if not exist %REPORT_DIR% mkdir %REPORT_DIR%
    
    REM 生成HTML报告
    jmeter -g %RESULTS_FILE% -o %REPORT_DIR%
    
    if %errorlevel% equ 0 (
        echo 测试报告生成成功，位于 %REPORT_DIR% 目录中
        echo 打开报告: %REPORT_DIR%\index.html
    ) else (
        echo 生成测试报告时出错
    )
) else (
    echo 测试执行过程中出错，请检查 jmeter.log 文件获取详细信息
)

pause