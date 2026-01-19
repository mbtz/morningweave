# MorningWeave platform setup

This guide walks through adding each platform, collecting the required keys,
and storing secrets safely.

## General flow
1. Initialize config: `morningweave init`
2. Enable a platform: `morningweave add-platform <name>`
3. Store credentials: `morningweave auth set <name>`
4. Review: `morningweave status`

Secrets can be stored in the OS keychain or 1Password (preferred) or in
`secrets.values` in `config.yaml` as a fallback.

## Email provider (Resend or SMTP)
1. Pick a provider and update `email.provider` in `config.yaml` if needed.
2. Store credentials:
   - Resend: `morningweave auth set email --value "<resend-api-key>"`
   - SMTP: `morningweave auth set email --value "<smtp-password>"`
3. Confirm the config fields:
   - Resend uses `email.resend.api_key_ref`.
   - SMTP uses `email.smtp.password_ref`.
4. Send a test: `morningweave test-email`

## Reddit
1. Create a Reddit app (type: script) at https://www.reddit.com/prefs/apps.
2. Note the client id, client secret, and a user agent string.
3. Required scope: `read`.
4. Store credentials:
   - Example payload:
     `{"client_id":"...","client_secret":"...","user_agent":"...","username":"...","password":"..."}`
   - Command:
     `morningweave auth set reddit --value '{"client_id":"...","client_secret":"...","user_agent":"...","username":"...","password":"..."}'`
5. Add sources with `morningweave add-platform reddit` or edit
   `platforms.reddit.sources` in `config.yaml`.

## X (x.com)
1. Create an app in the X developer portal.
2. Generate a bearer token.
3. Required scopes: `tweet.read`, `users.read`.
4. Store credentials:
   - Example payload: `{"bearer_token":"..."}`
   - Command: `morningweave auth set x --value '{"bearer_token":"..."}'`
5. Add sources with `morningweave add-platform x` or edit
   `platforms.x.sources` in `config.yaml`.

## Instagram
1. Ensure the Instagram account is Business or Creator and linked to a Facebook
   Page/app.
2. Enable Instagram Graph API in the Facebook app and generate an access token.
3. Required scopes: `instagram_basic`, `pages_show_list`,
   `instagram_manage_insights`.
4. Store credentials:
   - Example payload: `{"access_token":"...","user_id":"..."}`
   - Command: `morningweave auth set instagram --value '{"access_token":"...","user_id":"..."}'`
5. Add sources with `morningweave add-platform instagram` or edit
   `platforms.instagram.sources` in `config.yaml`.

## Hacker News
1. No API key required.
2. Configure sources under `platforms.hn.sources` in `config.yaml`.

## Verify
Run `morningweave status` to confirm enabled platforms and check for warnings.
