# Email Example

This example shows how to configure Event Reactor to send an email with a dynamically generated subject and body using go templating and data from the events payload.

## Yaml Configuration

Event Reactor needs to know what to react to. The sample configuration tells ER to send an email to `test@test.com` only when the event data has an attribute named `sendMail` and it's value is `true`. In addition, the example shows how to get a password for the SMTP relay server from a GCP secret

```yaml
reactorConfigs:
- name: test_email
  celExpressionFilter: attributes.sendMail == 'true'
  type: email
  properties:
    subject:
      value: '{{ .data.subject }}'
    from:
      value: someone@somewhere.com
    password:
      valueFrom:
        secretKeyRef:
          name: some_secret_for_email
          projectId: some-gcp-project
          version: latest
    to:
      value: test@test.com
    body:
      value: 'This is a test email from event reactor. Cool Info: {{ .data.coolMessage }}'
    smtpHost:
      value: smpt.com
    smtpPort:
      value: "587"
    maxRetries:
      value: "5"
```

The yaml must be saved as a file and placed on the file system. Once the file exists, the following command can be used to start ER using that configuration

```bash
er run server --config-file-path <path to yaml file>
```

## Event Payload
