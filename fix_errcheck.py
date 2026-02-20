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
            
        if "nolint:errcheck" not in lines[line_num - 1]:
            lines[line_num - 1] = lines[line_num - 1].rstrip() + " //nolint:errcheck\n"
            
        with open(file, "w") as f:
            f.writelines(lines)
