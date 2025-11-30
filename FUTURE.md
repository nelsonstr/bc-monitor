# Future Enhancements for bc-monitor

This document outlines potential future enhancements for the bc-monitor application, categorized by priority and structured into short-term roadmap and long-term vision.

## Short-Term Roadmap (3-6 Months)

Focus on enhancing core reliability, observability, and expanding blockchain coverage to deliver immediate value to users.

- **Data Persistence**: Implement database storage for transaction history and wallet data.
- **Monitoring and Metrics**: Add Prometheus metrics for comprehensive monitoring.
- **Testing Improvements**: Expand unit and integration test coverage.
- **Additional EVM Blockchain Support**: Add support for Polygon, BSC, and Avalanche.

## Long-Term Vision (6-12 Months+)

Aim for a more user-friendly, scalable, and secure platform that supports enterprise use cases.

- **User Interface**: Develop a web dashboard for monitoring and management.
- **Advanced Scalability**: Implement distributed architecture for horizontal scaling.
- **Enhanced Security**: Add authentication, encryption, and compliance features.
- **Broader Blockchain Support**: Extend to non-EVM chains like Cardano and Polkadot.

## Prioritized Features

### High Priority

1. **Data Persistence**
   - **Description**: Integrate a database (e.g., PostgreSQL) to store transaction history, wallet balances, and monitored addresses persistently.
   - **Benefits**: Enables historical data analysis, reduces reliance on external APIs, improves downtime recovery, and supports user queries for past transactions.
   - **Implementation Considerations**: Use GORM for ORM, add migration scripts, update models to include DB fields, ensure backward compatibility with in-memory storage.

2. **Prometheus Metrics**
   - **Description**: Implement comprehensive metrics collection including transaction counts, error rates, latency, and blockchain connectivity status.
   - **Benefits**: Provides real-time insights into system health, enables proactive alerting, and supports performance optimization.
   - **Implementation Considerations**: Integrate Prometheus Go client, expose /metrics endpoint, add custom metrics for each blockchain monitor.

3. **Unit and Integration Testing**
   - **Description**: Achieve 80%+ code coverage with comprehensive unit tests and integration tests for RPC interactions.
   - **Benefits**: Increases code reliability, reduces bugs in production, facilitates refactoring, and builds confidence in deployments.
   - **Implementation Considerations**: Use Go's testing framework, add mocks for external APIs (e.g., httptest), integrate with CI/CD for automated testing.

4. **Additional EVM Blockchain Support**
   - **Description**: Add monitoring support for popular EVM-compatible chains like Polygon, Binance Smart Chain, and Avalanche.
   - **Benefits**: Expands market coverage, attracts more users from diverse blockchain ecosystems, increases product value.
   - **Implementation Considerations**: Leverage existing EVM monitor module, add chain-specific configurations, update validation for new address formats.

### Medium Priority

5. **Web Dashboard UI**
   - **Description**: Create a simple web interface to view monitored addresses, recent transactions, and system health status.
   - **Benefits**: Improves usability for non-technical users, provides real-time visibility, enhances user experience.
   - **Implementation Considerations**: Use Gin framework for API, basic HTML/CSS/JS frontend, secure endpoints with authentication.

6. **Load Testing**
   - **Description**: Develop and execute load tests to validate performance under high transaction volumes and concurrent users.
   - **Benefits**: Ensures scalability, identifies bottlenecks, supports capacity planning.
   - **Implementation Considerations**: Use tools like k6 or Vegeta, simulate realistic loads, measure response times and resource usage.

7. **Enhanced Security**
   - **Description**: Implement API key encryption, user authentication, and audit logging.
   - **Benefits**: Protects sensitive data, ensures compliance with security standards, builds trust with enterprise users.
   - **Implementation Considerations**: Use Go crypto libraries for encryption, add JWT-based auth, implement structured audit logs.

### Low Priority

8. **Non-EVM Blockchain Support**
   - **Description**: Add monitoring for non-EVM chains such as Cardano and Polkadot.
   - **Benefits**: Further expands market reach, positions as comprehensive multi-chain monitor.
   - **Implementation Considerations**: Develop new monitor modules, research chain-specific APIs, ensure modular architecture supports extension.

9. **Distributed Architecture**
   - **Description**: Support multiple application instances with load balancing and clustering.
   - **Benefits**: Enables horizontal scaling, improves fault tolerance, supports high-availability deployments.
   - **Implementation Considerations**: Use Kubernetes for orchestration, implement leader election, distribute monitoring workloads.

10. **Advanced Monitoring (Grafana Dashboards)**
    - **Description**: Create Grafana dashboards for visualizing Prometheus metrics.
    - **Benefits**: Provides intuitive visual insights, supports data-driven decision making.
    - **Implementation Considerations**: Configure Grafana data sources, design dashboards for key metrics, integrate with alerting.
