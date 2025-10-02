class Person:
    """
    定义一个名为Person的类，包含属性name和age，并定义一个方法display，用于打印对象的信息。
    """
    def __init__(self, name, age):
        self.name = name
        self.age = age
    def bark(self):
        return "汪汪汪"

    def display(self):
        return f"我的名字是{self.name}，今年{self.age}岁"
buddy = Person("buddy", 5)
print(buddy.display())