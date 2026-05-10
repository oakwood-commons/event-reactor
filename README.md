# event-reactor

The event-reactor is an API designed to listen to and react to events from Google Cloud Pub/Sub. It provides a robust set of features that allow for a wide range of responses to these events, making it a versatile tool for managing and responding to data flow.

One of the key features of event-reactor is its ability to filter incoming events based on their attributes or payload. This is achieved using the Common Expression Language (CEL) developed by Google, which provides a simple and efficient way to manage event data.

The event-reactor API comes with a variety of built-in reactions to events. These include sending an email, creating a GitHub commit and pull request comment, executing a PowerShell command or script, and sending a webhook. Future updates will also add the ability to execute a bash command or script, send a pub/sub event, and create a Webex message.

In addition to these reactions, event-reactor also supports several methods for getting property data. These include static values, values from attributes or payloads (with support for multiple property paths), environment variables, files, and Google Cloud Platform (GCP). This flexibility allows for a wide range of data sources to be used in reactions.

The API also supports the use of Go templating to transform property values, providing further flexibility in how data is handled and used. In the future, event-reactor will also support extensions, allowing for the addition of new reactions beyond those built into the API.

In summary, event-reactor is a powerful and flexible API for managing and responding to GCP pub/sub events, with a wide range of features and capabilities.

## Feature List

- Ability to filter incoming events on attributes or the payload using [CEL](https://github.com/google/cel-go)
- Has the following built-in reactions
  - Send email
  - Creating a github commit and pull request comment
  - Execute a PowerShell command/script
  - Send a webhook
  - (Coming soon) Execute a bash command/script
  - (Coming soon) Send a pub/sub event
  - (Coming soon) Create a Webex message
- Supports getting property data in the following ways
  - Static value
  - Value from attributes or payload
    - Supports multiple property paths
  - From environment variable
  - From a file
  - From GCP
- Supports the use of go templating to transform the property values
- (Coming soon) Supports extensions to extend the reactions it supports
- echo endpoint for testing purposes

## Discovery

### List Reactors

Execute the command to get a list of all the available reactors

```bash
er get reactor
```

> Example Output

```bash
EMAIL: This reactor sends an email to the specified recipient(s) using the supplied smtp server and credentials. The email subject and body can be templated using Go's text/template package. The email is retried up to the specified number of times if it fails to send.

POWERSHELL: This reactor executes a powershell command. The parameter values support go templating.

WEBHOOK: This reactor sends a webhook to a specified URL. The payload of the webhook is the event data.

GITHUB/COMMENT: This reactor writes comments on commits and pull requests. It requires a GitHub token with appropriate permissions to interact with the specified repository. Key inputs include the organization, repository, commit SHA, and pull request number. One of the standout features of this reactor is its support for Go templating, which can be used to customize the heading and body of the comments. The heading also plays a crucial role in identifying previous comments for deletion. Moreover, the reactor offers a suite of configuration options for enhanced control. These include the ability to purge existing comments from all commits associated with a pull request, remove comments from the pull request itself, and eliminate duplicate commit comments.
```

### Get Reactor Details

Execute the command to get a list of all the available reactors

```bash
er get reactor --name email
```

> Example Output

```bash
NAME
  email

DESCRIPTION
  This reactor sends an email to the specified recipient(s) using the supplied smtp server and credentials. The email subject and body can be templated using Go's text/template package. The email is retried up to the specified number of times if it fails to send.

PROPERTIES

  from
    The email address of the sender

    Required:                 true
    Type:                     string


  password
    The password for the smtp server

    Required:                 true
    Type:                     string


  to
    The email address of the recipient(s). Multiple addresses can be separated by a comma, semicolon, or space

    Required:                 true
    Type:                     string


  subject
    The subject of the email. This field supports go templating

    Required:                 true
    Type:                     string


  body
    The body of the email. This field supports go templating

    Required:                 true
    Type:                     string


  smtpHost
    The smtp server host

    Required:                 true
    Type:                     string


  smtpPort
    The smtp server port

    Required:                 true
    Type:                     string


  maxRetries
    The maximum number of times to retry sending the email. Defaults to 5

    Required:                 false
    Type:                     string


EXAMPLE CONFIG

  reactorConfigs:
  - name: test_email
    celExpressionFilter: attributes.test == 'email'
    type: email
    properties:
      subject:
        value: Testing email
      from:
        value: someone@somewhere.com
      password:
        valueFrom:
          secretKeyRef:
            name: some_secret_for_email
            projectId: some-gcp-project
            version: latest
      to:
        value: someoneelse@somewherelese.com
      body:
        value: this is a test email from event reactor
      smtpHost:
        value: smpt.com
      smtpPort:
        value: "587"
      maxRetries:
        value: "5"
```


## Links

- [protobuf/avro payload integration](https://cloud.google.com/pubsub/docs/samples/pubsub-subscribe-avro-records-with-revisions?hl=en)