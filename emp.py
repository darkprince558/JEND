class Employee:
    def __init__(self, name, email, salary):
        self.name = name
        self.email = email
        self.salary = salary

    def printEmployee(self):
        print(self.name, self.email, self.salary)


if __name__ == "__main__":
    print("This module was executed")
