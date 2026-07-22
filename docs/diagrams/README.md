# Diagrams

The PlantUML files in this directory describe the current KiTicket implementation. They are source files, not generated evidence of a production topology. No binary exports are committed, so a diagram cannot drift silently from its editable source.

| File | Scope |
|---|---|
| component-diagram.puml | React client, modular API and its three runtime dependencies |
| booking-sequence.puml | waiting-room admission, reservation, sandbox payment and ticket issuance |
| class-diagram.puml | persisted TypeORM entities and key unique constraints |
| local-deployment.puml | Docker Compose services and exposed ports |

Render them with a local PlantUML installation, for example:

~~~
java -jar /path/to/plantuml.jar -tsvg docs/diagrams/*.puml
~~~

Review a rendered diagram whenever its associated controller, entity or Compose configuration changes.

