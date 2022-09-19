# build

This directory contains [Cloud Build configuration files] for deploying the App
Engine app and running tests using Google's [Cloud Build] service.

[Cloud Build configuration files]: https://cloud.google.com/build/docs/build-config-file-schema
[Cloud Build]: https://cloud.google.com/build

## Deploying

[deploy_app.yaml](./deploy_app.yaml) and
[deploy_indexes.yaml](./deploy_indexes.yaml) deploy the App Engine app and
update Datastore indexes, respectively.

Through a painful process of trial and error, I've found that the following
steps seem to allow Cloud Build triggers in one GCP project (e.g. `my-build`
with a `123@cloudbuild.gserviceaccount.com` service account) to deploy the App
Engine app in a second GCP project (e.g. `my-app`):

On the `my-app@appspot.gserviceaccount.com` service account page, add
`123@cloudbuild.gserviceaccount.com` as a principal with the `Service Account
User` role.

On the `my-app` IAM page, grant the `123@cloudbuild.gserviceaccount.com`
principal the following roles:

*   App Engine Admin
*   Cloud Build Editor
*   Cloud Datastore Index Admin
*   Container Registry Service Account

On the `foo.appspot.com` and `staging.foo.appspot.com` Cloud Storage bucket
permission pages, grant `123@cloudbuild.gserviceaccount.com` the `Storage Object
Admin` role.

I couldn't get [service account impersonation] to work at all.

[service account impersonation]: https://cloud.google.com/iam/docs/impersonating-service-accounts

## Testing

[test.yaml](./test.yaml) runs `go test ./...`.

[Dockerfile](./Dockerfile) is used to build a [Docker] container image with Go,
Chrome, the Google Cloud SDK, and related dependencies preinstalled for running
tests. When executed in this directory, the following command uses Cloud Build
to build a container and submit it to the [Container Registry].

```
gcloud --project ${PROJECT_ID} builds submit \
  --tag gcr.io/${PROJECT_ID}/nup-test --timeout=20m
```

[Docker]: https://www.docker.com/
[Container Registry]: https://cloud.google.com/container-registry
