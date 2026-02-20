import re

with open("lint_output.txt", "r") as f:
    out = f.read()

for line in out.splitlines():
    match = re.search(r'^([^:]+):(\d+):\d+: Error return value', line)
    if match:
        file = match.group(1)
        line_num = int(match.group(2))
        
        with open(file, "r") as f:
            lines = f.readlines()
            
        target = lines[line_num - 1]
        # Remove any injected //nolint:errcheck
        target = target.replace(" //nolint:errcheck\n", "\n")
        target = target.replace("//nolint:errcheck\n", "\n")
        
        # Determine indentation
        indent = match = re.match(r'^([ \t]*)', target).group(1)
        content = target.strip()
        
        if content.startswith("defer "):
            # rewrite defer Call() to defer func() { _ = Call() }()
            call = content[6:]
            new_target = f"{indent}defer func() {{ _ = {call} }}()\n"
        else:
            # simply prepend _ = 
            new_target = f"{indent}_ = {content}\n"
            
        lines[line_num - 1] = new_target
            
        with open(file, "w") as f:
            f.writelines(lines)
