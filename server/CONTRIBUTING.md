# Contributing to Quartz

Welcome! We are excited to have you contribute to Quartz. To keep our development fast and our production environment safe, we strictly use **GitHub Flow** (Trunk-Based Development).

Please follow this cheat sheet for all contributions.

---

## 🚀 The Development Workflow (Cheat Sheet)

We do **not** use a `dev` branch. `master` is our single source of truth and must always contain working, deployable code.

### 1. Start a New Task
Never work directly on `master`. Always create a new, short-lived branch for the specific feature or bug you are fixing.
```bash
# Ensure you have the latest code
git checkout master
git pull origin master

# Create your feature branch (e.g., feature/login-page, bugfix/header-padding)
git checkout -b feature/<your-feature-name>
```

### 2. Write Code & Push
Do your work, commit it, and push your branch to GitHub.
```bash
git add .
git commit -m "Add descriptive message of what you did"

# Push to GitHub
git push -u origin feature/<your-feature-name>
```

### 3. Open a Pull Request (PR)
Go to GitHub and open a Pull Request from your feature branch into `master`.
*   Our automated CI pipeline (`pr.yml`) will run tests to ensure your code compiles.
*   Wait for a maintainer to review and approve your code.

### 4. Squash & Merge
When your PR is approved, the maintainer (or you) will click **Squash and merge**.
*   This crushes all your messy commits down into one clean commit on `master`.

### 5. Clean Up
Once merged, your branch is no longer needed. Delete it locally to keep your environment clean!
```bash
git checkout master
git pull origin master
git branch -d feature/<your-feature-name>
```

---

## 🛠 Fixing Merge Conflicts (The Safe Way)
If someone else merges code into `master` while you are still working on your branch, you might get a conflict. **Never use `rebase` unless you are a Git expert.** Instead, use a standard merge:

```bash
# 1. Update your local master
git checkout master
git pull origin master

# 2. Go back to your feature branch
git checkout feature/<your-feature-name>

# 3. Safely merge master into your branch
git merge master
```
Fix the conflicts in your editor, commit them, and push. Your PR will update automatically!
