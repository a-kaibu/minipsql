FROM maven:3.9-eclipse-temurin-21 AS build

WORKDIR /src
COPY external/sqlancer/ ./
RUN mvn -q -DskipTests package

FROM eclipse-temurin:21-jre

WORKDIR /work
COPY --from=build /src/target/sqlancer-*.jar /opt/sqlancer/sqlancer.jar
COPY --from=build /src/target/lib/ /opt/sqlancer/lib/

ENTRYPOINT ["java", "-jar", "/opt/sqlancer/sqlancer.jar"]
