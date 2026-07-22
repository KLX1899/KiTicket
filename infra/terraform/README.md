# Terraform deployment

This stack provisions a two-AZ VPC, private autoscaled application instances, a public ALB,
encrypted managed PostgreSQL and persistent Redis. The default single NAT gateway keeps the
course environment affordable; set `single_nat_gateway=false` for AZ-level production resilience.

```bash
terraform init
terraform validate
terraform plan \
  -var='backend_container_image=registry.example/ticketing-backend:1.0' \
  -var='db_password=URL_SAFE_SECRET' \
  -var='jwt_secret=AT_LEAST_32_RANDOM_CHARACTERS'
```

Never commit a `.tfvars` file containing secrets. Use the CI secret store or
`TF_VAR_db_password`/`TF_VAR_jwt_secret`. This stack intentionally does not run `apply` in CI.
