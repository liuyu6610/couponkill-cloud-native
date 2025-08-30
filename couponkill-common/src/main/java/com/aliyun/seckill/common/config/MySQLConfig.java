// MySQLConfig.java
// 文件路径: com/aliyun/seckill/common/config/MySQLConfig.java
package com.aliyun.seckill.common.config;

import com.zaxxer.hikari.HikariConfig;
import com.zaxxer.hikari.HikariDataSource;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

import javax.sql.DataSource;
import java.util.List;

@Configuration
public class MySQLConfig {

    // 单节点配置
    @Value("${spring.datasource.host:mysql}")
    private String host;
    @Value("${spring.datasource.port:3306}")
    private int port;
    @Value("${spring.datasource.database:couponkill}")
    private String database;
    @Value("${spring.datasource.username:root}")
    private String username;
    @Value("${spring.datasource.password:password}")
    private String password;

    // 集群配置
    @Value("${spring.datasource.cluster.nodes:}")
    private List<String> clusterNodes;

    // 主从复制配置
    @Value("${spring.datasource.replication.master.host:}")
    private String masterHost;
    @Value("${spring.datasource.replication.master.port:3306}")
    private int masterPort;

    @Value("${spring.datasource.replication.slaves:}")
    private List<String> slaveNodes;

    /**
     * 创建主数据源
     */
    @Bean
    public DataSource masterDataSource() {
        HikariConfig config = new HikariConfig();
        config.setJdbcUrl(String.format("jdbc:mysql://%s:%d/%s", masterHost, masterPort, database));
        config.setUsername(username);
        config.setPassword(password);
        config.setDriverClassName("com.mysql.cj.jdbc.Driver");
        
        // 连接池配置
        config.setMaximumPoolSize(20);
        config.setMinimumIdle(5);
        config.setConnectionTimeout(30000);
        config.setIdleTimeout(600000);
        config.setMaxLifetime(1800000);
        
        return new HikariDataSource(config);
    }

    /**
     * 创建从数据源
     */
    @Bean
    public DataSource slaveDataSource() {
        if (slaveNodes == null || slaveNodes.isEmpty()) {
            // 如果没有配置从节点，返回主数据源
            return masterDataSource();
        }
        
        // 使用第一个从节点作为示例
        String[] parts = slaveNodes.get(0).split(":");
        String slaveHost = parts[0];
        int slavePort = parts.length > 1 ? Integer.parseInt(parts[1]) : 3306;
        
        HikariConfig config = new HikariConfig();
        config.setJdbcUrl(String.format("jdbc:mysql://%s:%d/%s", slaveHost, slavePort, database));
        config.setUsername(username);
        config.setPassword(password);
        config.setDriverClassName("com.mysql.cj.jdbc.Driver");
        
        // 连接池配置
        config.setMaximumPoolSize(20);
        config.setMinimumIdle(5);
        config.setConnectionTimeout(30000);
        config.setIdleTimeout(600000);
        config.setMaxLifetime(1800000);
        
        return new HikariDataSource(config);
    }

    /**
     * 根据配置模式创建数据源
     */
    public DataSource createDataSource(String mode, List<String> nodes, String username, String password, String database) {
        switch (mode.toLowerCase()) {
            case "cluster":
                // 集群模式 - 使用第一个节点作为示例
                if (nodes != null && !nodes.isEmpty()) {
                    String[] parts = nodes.get(0).split(":");
                    String host = parts[0];
                    int port = parts.length > 1 ? Integer.parseInt(parts[1]) : 3306;
                    
                    HikariConfig clusterConfig = new HikariConfig();
                    clusterConfig.setJdbcUrl(String.format("jdbc:mysql://%s:%d/%s", host, port, database));
                    clusterConfig.setUsername(username);
                    clusterConfig.setPassword(password);
                    clusterConfig.setDriverClassName("com.mysql.cj.jdbc.Driver");
                    return new HikariDataSource(clusterConfig);
                }
                break;
                
            case "replication":
                // 主从复制模式 - 使用主节点
                if (nodes != null && !nodes.isEmpty()) {
                    String[] parts = nodes.get(0).split(":");
                    String host = parts[0];
                    int port = parts.length > 1 ? Integer.parseInt(parts[1]) : 3306;
                    
                    HikariConfig masterConfig = new HikariConfig();
                    masterConfig.setJdbcUrl(String.format("jdbc:mysql://%s:%d/%s", host, port, database));
                    masterConfig.setUsername(username);
                    masterConfig.setPassword(password);
                    masterConfig.setDriverClassName("com.mysql.cj.jdbc.Driver");
                    return new HikariDataSource(masterConfig);
                }
                break;
                
            case "standalone":
            default:
                // 单节点模式
                HikariConfig standaloneConfig = new HikariConfig();
                standaloneConfig.setJdbcUrl(String.format("jdbc:mysql://%s:%d/%s", host, port, database));
                standaloneConfig.setUsername(username);
                standaloneConfig.setPassword(password);
                standaloneConfig.setDriverClassName("com.mysql.cj.jdbc.Driver");
                return new HikariDataSource(standaloneConfig);
        }
        
        // 默认返回单节点数据源
        HikariConfig defaultConfig = new HikariConfig();
        defaultConfig.setJdbcUrl(String.format("jdbc:mysql://%s:%d/%s", host, port, database));
        defaultConfig.setUsername(username);
        defaultConfig.setPassword(password);
        defaultConfig.setDriverClassName("com.mysql.cj.jdbc.Driver");
        return new HikariDataSource(defaultConfig);
    }
}