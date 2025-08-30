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
set LOG_FILE=jmeter.log

echo 开始执行性能测试...
echo 测试计划: %TEST_PLAN%
echo 结果文件: %RESULTS_FILE%
echo 日志文件: %LOG_FILE%

REM 执行JMeter测试
jmeter -n -t %TEST_PLAN% -l %RESULTS_FILE% -j %LOG_FILE%

if %errorlevel% equ 0 (
    echo.
    echo 测试执行完成，正在生成测试报告...
    echo 结果文件: %RESULTS_FILE%
    echo 报告目录: %REPORT_DIR%
    
    REM 删除旧的报告目录
    if exist %REPORT_DIR% (
        echo 删除旧的报告目录...
        rmdir /s /q %REPORT_DIR%
    )
    
    REM 创建新的报告目录
    mkdir %REPORT_DIR%
    
    REM 生成HTML报告
    jmeter -g %RESULTS_FILE% -o %REPORT_DIR%
    
    if %errorlevel% equ 0 (
        echo.
        echo ================================
        echo 测试报告生成成功！
        echo 报告位置: %REPORT_DIR%\index.html
        echo ================================
        echo 打开报告...
        start "" "%REPORT_DIR%\index.html"
    ) else (
        echo.
        echo 生成测试报告时出错，请检查日志文件
    )
) else (
    echo.
    echo 测试执行过程中出错，请检查 %LOG_FILE% 文件获取详细信息
)

echo.
echo 测试脚本执行完毕
pause