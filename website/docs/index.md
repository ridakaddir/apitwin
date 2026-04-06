---
layout: home

hero:
  name: apitwin
  text: Mock, stub, and proxy APIs
  tagline: A fast, zero-dependency CLI tool for developers to mock, stub, and proxy HTTP and gRPC APIs — written in Go.
  image:
    src: /logo.svg
    alt: apitwin
  actions:
    - theme: brand
      text: Get Started
      link: /installation
    - theme: alt
      text: Quick Start
      link: /quick-start
    - theme: alt
      text: View on GitHub
      link: https://github.com/ridakaddir/apitwin

features:
  - icon: ⚡
    title: Zero Dependencies
    details: No external dependencies on your app. Just point your frontend or service at apitwin instead of the real API.

  - icon: 🔄
    title: Instant Hot Reload
    details: Switch between response scenarios by editing a config file — changes apply instantly with no restart.

  - icon: 🎯
    title: Selective Mocking
    details: Mock only the endpoints you're actively building. Forward everything else to the real backend.

  - icon: 📁
    title: Directory-Based Stubs
    details: CRUD operations with one JSON file per resource. Perfect for realistic data management during development.

  - icon: 🌐
    title: HTTP & gRPC Support
    details: Full support for both HTTP/REST APIs and gRPC services with automatic proto generation.

  - icon: 🔗
    title: Cross-Endpoint References
    details: Reference data from other stub files with filtering and transformation using the <code v-pre>{{ref:...}}</code> syntax.

  - icon: 🔁
    title: Array Processing
    details: Iterate over collections and reshape each item with $each / $template syntax for building complex interconnected responses.

  - icon: ⏰
    title: Response Transitions
    details: Time-based state progression across response cases for testing different application states.

  - icon: 🎨
    title: Template Tokens
    details: Dynamic content generation with <code v-pre>{{uuid}}</code>, <code v-pre>{{now}}</code>, <code v-pre>{{timestamp}}</code> and other template tokens.

  - icon: 📝
    title: OpenAPI Generation
    details: Generate complete mock APIs from OpenAPI 3 specifications with high-quality stub synthesis.

  - icon: 📊
    title: Record Mode
    details: Proxy a real API, save responses automatically, then replay them offline for consistent testing.
---
