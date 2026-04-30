---
customInstructions:
  role: >-
    Documentation standards for JAAS homepage and landing pages. When asked to update landing pages,
    FIRST assess the current state against these standards, identify gaps, and create a todo list
    grouped by format and content issues. ALWAYS present assessment to user before making changes.
applyTo:
  - pattern: "**/index.md"
    reason: Homepage and landing pages follow specific structure and content patterns
  - pattern: "**/tutorial/index.md"
    reason: Tutorial landing page should group and explain content flow
  - pattern: "**/howto/index.md"
    reason: How-to landing page should organize by lifecycle/workflow
  - pattern: "**/reference/index.md"
    reason: Reference landing page should organize by dependency flow and architectural layers
  - pattern: "**/explanation/index.md"
    reason: Explanation landing page should group related conceptual topics
---

# Landing Pages Documentation Standards for JAAS

## Homepage Intro Paragraphs

**Standard structure**: Four brief paragraphs covering:

1. **What the product is** - Succinct, memorable definition
2. **What the product does** - Core capabilities in plain language  
3. **What problem the product solves** - The need it meets
4. **Who the product is for** - Target audience

**Template example**:
```markdown
Something that says what the product is, succinctly and memorably consectetur adipiscing elit, sed do eiusmod tempor.

A description of what the product does. Urna cursus eget nunc scelerisque viverra mauris in. Nibh mauris cursus mattis molestie a iaculis at vestibulum rhoncus est pellentesque elit.

An account of what need the product meets. Dui ut ornare lectus sit amet lam.

Something that describes whom the product is useful for. Nunc non blandit massa enim nec dui nunc mattis enim.
```

**Key principles**:
- Keep each paragraph brief (1-2 sentences maximum)
- Avoid technical jargon in favor of clear benefits
- Make the first sentence memorable and quotable
- End with a clear call to value for the target audience

## Organizing Principle: Dependency Flow and Architectural Layers

**Core principle**: Organize content following the dependency chain and architectural layers specific to JAAS.

### JAAS Architecture and Dependency Flow

JAAS extends Juju by adding multi-controller coordination and enhanced authentication/authorization. The dependency flow is:

**JAAS Plugin** → **Multi-controller Coordination** → **Enhanced Entities**

1. **JAAS Plugin** (interface layer)
   - How you interact with JAAS
   - Extends the `juju` CLI with JAAS-specific commands

2. **Multi-controller Coordination** (control plane layer)
   - How JAAS manages multiple Juju controllers
   - Authentication (Candid, OAuth)
   - Authorization (OpenFGA, ReBAC)
   - Controller aggregation and proxying

3. **Enhanced Entities** (resource layer)
   - Juju entities with JAAS enhancements (cloud*, controller*, model*, user*, offer*)
   - JAAS-specific entities (group, role)

### Applying This to Landing Pages

**Reference landing page** (`docs/reference/index.md`):
Organize sections to reflect the architectural layers:

```markdown
## JAAS Tools
(Interface layer - how you interact)
- JAAS plugin, audit logs

## JAAS Entities
(Resource layer - what you manage, grouped by type)
- Enhanced Juju entities (cloud*, controller*, model*, offer*, user*)
- JAAS-specific entities (group, role)

## JAAS as a whole
(System layer - how it works internally)
- JAAS architecture, supported Juju versions
```

**Explanation landing page** (`docs/explanation/index.md`):
Organize by understanding hierarchy:

```markdown
## JAAS at a glance
(Foundational understanding - architecture, security model)
- Reference architecture, Architecture, Authentication, Authorization, Security

## JAAS tooling
(Tool-specific concepts)
- JAAS plugin specifics
```

**How-to landing page** (`docs/howto/index.md`):
Organize by operational lifecycle:

```markdown
## Set up JAAS
(Initial setup and configuration)

## Manage authentication and authorization
(User and permission management)

## Manage controllers and models
(Day-to-day operations)

## Monitor and troubleshoot
(Operational visibility)
```

### Why This Organization Works for JAAS

- **Matches user mental model**: Users think "I use the plugin to manage enhanced entities through JAAS coordination"
- **Shows value proposition**: Makes clear what JAAS adds on top of Juju
- **Scales logically**: New features fit into existing layers
- **Architecture-aligned**: Reflects how JAAS is actually built

## Quality Checklist for JAAS Landing Pages

**Homepage format**:
- [ ] "In this documentation" section with bullet points organized by domain
- [ ] "How this documentation is organised" section - use EITHER text format OR Diátaxis quadrant grid, not both
- [ ] "Project and community" section with subsections (Get involved, Releases, Governance)
- [ ] NO horizontal lines (`---`) used for section separators

**Reference**:
- [ ] Tools section lists interface/interaction methods
- [ ] Entities section distinguishes enhanced (*) from JAAS-native
- [ ] "JAAS as a whole" covers system architecture
- [ ] Each section has explanatory text connecting to JAAS's role

**Explanation**:
- [ ] "At a glance" covers foundational architecture/security concepts
- [ ] Progression from fundamental to specific is clear
- [ ] Links to upstream Juju concepts where JAAS builds on them

**How-to**:
- [ ] Grouped by operational workflow stages
- [ ] Setup → Auth/authz → Day-to-day → Monitoring progression is clear
- [ ] Cross-references to Juju docs for non-JAAS-specific operations
